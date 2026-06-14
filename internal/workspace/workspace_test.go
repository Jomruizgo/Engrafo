package workspace_test

import (
	"path/filepath"
	"testing"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
	"github.com/Jomruizgo/Engrafo/v2/internal/workspace"
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

// TestResolveFileToRoot â€” test #6: prefijo mÃ¡s largo gana; path fuera de toda raÃ­z â†’ ok=false.
func TestResolveFileToRoot(t *testing.T) {
	s := openStore(t)

	// Registrar dos raÃ­ces con paths anidados (el mÃ¡s largo debe ganar).
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
			name:        "prefijo mÃ¡s largo gana",
			absPath:     filepath.FromSlash("/repos/ws/backend/pkg/auth.go"),
			wantRoot:    "backend",
			wantRelPath: "pkg/auth.go",
			wantOK:      true,
		},
		{
			name:        "raÃ­z padre cuando no hay raÃ­z hija que matchee",
			absPath:     filepath.FromSlash("/repos/ws/shared/util.go"),
			wantRoot:    "workspace",
			wantRelPath: "shared/util.go",
			wantOK:      true,
		},
		{
			name:    "path fuera de toda raÃ­z â†’ ok=false",
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
