package workspace_test

import (
	"path/filepath"
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/workspace"
)

func openStore(t *testing.T) *graph.Store {
	t.Helper()
	s, err := graph.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestResolveFileToRoot — test #6: prefijo más largo gana; path fuera de toda raíz → ok=false.
func TestResolveFileToRoot(t *testing.T) {
	s := openStore(t)

	// Registrar dos raíces con paths anidados (el más largo debe ganar).
	_, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "workspace", RelPath: ".", AbsRoot: "/repos/ws", VCS: "none",
	})
	if err != nil {
		t.Fatalf("UpsertRoot workspace: %v", err)
	}
	_, err = s.UpsertRoot(graph.ResolvedRoot{
		Name: "backend", RelPath: "backend", AbsRoot: "/repos/ws/backend", VCS: "git",
	})
	if err != nil {
		t.Fatalf("UpsertRoot backend: %v", err)
	}

	tests := []struct {
		name        string
		absPath     string
		wantRoot    string
		wantRelPath string
		wantOK      bool
	}{
		{
			name:        "prefijo más largo gana",
			absPath:     filepath.FromSlash("/repos/ws/backend/pkg/auth.go"),
			wantRoot:    "backend",
			wantRelPath: "pkg/auth.go",
			wantOK:      true,
		},
		{
			name:        "raíz padre cuando no hay raíz hija que matchee",
			absPath:     filepath.FromSlash("/repos/ws/shared/util.go"),
			wantRoot:    "workspace",
			wantRelPath: "shared/util.go",
			wantOK:      true,
		},
		{
			name:    "path fuera de toda raíz → ok=false",
			absPath: filepath.FromSlash("/other/project/main.go"),
			wantOK:  false,
		},
		{
			name:    "prefijo accidental no matchea (ws vs workspace)",
			absPath: filepath.FromSlash("/repos/ws-extra/file.go"),
			wantOK:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rootName, relPath, ok := workspace.ResolveFileToRoot(s, tc.absPath)
			if ok != tc.wantOK {
				t.Fatalf("ok: want %v, got %v", tc.wantOK, ok)
			}
			if !tc.wantOK {
				return
			}
			if rootName != tc.wantRoot {
				t.Errorf("rootName: want %q, got %q", tc.wantRoot, rootName)
			}
			if relPath != tc.wantRelPath {
				t.Errorf("relPath: want %q, got %q", tc.wantRelPath, relPath)
			}
		})
	}
}
