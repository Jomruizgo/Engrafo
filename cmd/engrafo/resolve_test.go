package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeManifest crea un engrafo.json mínimo en el directorio dado.
func writeManifest(t *testing.T, dir string) {
	t.Helper()
	manifest := map[string]any{
		"version": 1,
		"roots":   []any{},
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "engrafo.json"), data, 0644); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}
}

// writeGitDir crea un directorio .git mínimo para simular un repo git.
func writeGitDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatalf("writeGitDir: %v", err)
	}
}

// TestResolveDBAuto verifica la precedencia de resolveDB (test #5 del diseño):
//  1. manifest gana sobre .git
//  2. .git solo → usa git root
//  3. sin manifest ni .git → usa startDir (CWD fallback)
func TestResolveDBAuto(t *testing.T) {
	t.Run("manifest gana sobre git", func(t *testing.T) {
		// Estructura:
		//   wsDir/          ← engrafo.json aquí
		//     sub/repo/     ← .git aquí, startDir del test
		wsDir := t.TempDir()
		repoDir := filepath.Join(wsDir, "sub", "repo")
		if err := os.MkdirAll(repoDir, 0755); err != nil {
			t.Fatal(err)
		}
		writeManifest(t, wsDir)
		writeGitDir(t, repoDir)

		cfg := &config{}
		got, err := cfg.resolveDBFrom(repoDir)
		if err != nil {
			t.Fatalf("resolveDBFrom: %v", err)
		}
		wantDB := filepath.Join(wsDir, ".engrafo", "graph.db")
		if got != wantDB {
			t.Errorf("manifest debe ganar:\n  want %s\n   got %s", wantDB, got)
		}
		if cfg.workspaceDir != wsDir {
			t.Errorf("workspaceDir: want %s, got %s", wsDir, cfg.workspaceDir)
		}
		if cfg.manifestPath != filepath.Join(wsDir, "engrafo.json") {
			t.Errorf("manifestPath: want %s, got %s",
				filepath.Join(wsDir, "engrafo.json"), cfg.manifestPath)
		}
	})

	t.Run("git sin manifest", func(t *testing.T) {
		// Estructura:
		//   gitRoot/    ← .git aquí, startDir del test
		gitRoot := t.TempDir()
		writeGitDir(t, gitRoot)

		cfg := &config{}
		got, err := cfg.resolveDBFrom(gitRoot)
		if err != nil {
			t.Fatalf("resolveDBFrom: %v", err)
		}
		wantDB := filepath.Join(gitRoot, ".engrafo", "graph.db")
		if got != wantDB {
			t.Errorf("git root: want %s, got %s", wantDB, got)
		}
		if cfg.workspaceDir != "" {
			t.Errorf("workspaceDir must be empty when no manifest, got %q", cfg.workspaceDir)
		}
	})

	t.Run("sin manifest ni git", func(t *testing.T) {
		// Directorio aislado sin .git ni engrafo.json.
		isolated := t.TempDir()

		cfg := &config{}
		got, err := cfg.resolveDBFrom(isolated)
		if err != nil {
			t.Fatalf("resolveDBFrom: %v", err)
		}
		wantDB := filepath.Join(isolated, ".engrafo", "graph.db")
		if got != wantDB {
			t.Errorf("CWD fallback: want %s, got %s", wantDB, got)
		}
	})

	t.Run("manifest no se confunde con subdirectorio de git", func(t *testing.T) {
		// Manifest en wsDir, startDir es un subdirectorio debajo de wsDir
		// que también tiene .git — el manifest más cercano arriba manda.
		wsDir := t.TempDir()
		deepDir := filepath.Join(wsDir, "a", "b", "c")
		if err := os.MkdirAll(deepDir, 0755); err != nil {
			t.Fatal(err)
		}
		writeManifest(t, wsDir)

		cfg := &config{}
		got, err := cfg.resolveDBFrom(deepDir)
		if err != nil {
			t.Fatalf("resolveDBFrom: %v", err)
		}
		if !strings.HasPrefix(got, wsDir) {
			t.Errorf("db debe estar bajo wsDir=%s, got %s", wsDir, got)
		}
	})

	t.Run("--db flag siempre gana", func(t *testing.T) {
		wsDir := t.TempDir()
		writeManifest(t, wsDir)

		explicit := "/explicit/graph.db"
		cfg := &config{dbPath: explicit}
		got, err := cfg.resolveDBFrom(wsDir)
		if err != nil {
			t.Fatalf("resolveDBFrom: %v", err)
		}
		if got != explicit {
			t.Errorf("--db flag: want %s, got %s", explicit, got)
		}
	})
}
