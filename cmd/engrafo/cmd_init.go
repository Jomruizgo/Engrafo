package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
	"github.com/Jomruizgo/Engrafo/internal/parser/extractors"
)

// skipDirs are directory names that are never indexed.
var skipDirs = map[string]bool{
	".git": true, ".engrafo": true, "node_modules": true,
	"vendor": true, ".idea": true, ".vscode": true,
	"dist": true, "build": true, "__pycache__": true,
	"target": true, ".next": true, ".nuxt": true,
}

func newParser() *parser.Parser {
	return parser.New(
		&extractors.GoExtractor{},
		&extractors.TypeScriptExtractor{},
		&extractors.PythonExtractor{},
	)
}

func cmdInit(cfg *config, args []string) error {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	dbPath, dbErr := cfg.resolveDB()
	if dbErr != nil {
		dbPath = filepath.Join(root, ".engrafo", "graph.db")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("create .engrafo dir: %w", err)
	}

	s, err := graph.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer s.Close()

	commitHash := currentHEAD(root)

	p := newParser()
	b := graph.NewBuilder(s)

	var indexed, skipped int
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}

		lang := parser.Detect(path)
		if lang == "" {
			return nil
		}

		result, parseErr := p.ParseFile(path)
		if parseErr != nil {
			skipped++
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for i := range result.Nodes {
			result.Nodes[i].FilePath = rel
		}

		if upsertErr := b.UpsertFile(commitHash, result); upsertErr != nil {
			return fmt.Errorf("upsert %s: %w", rel, upsertErr)
		}
		indexed++
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	db := s.DB()
	db.Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('last_commit_hash',?)`, commitHash)
	db.Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('repo_root',?)`, root)
	db.Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('indexed_at',datetime('now'))`)

	fmt.Fprintf(cfg.stdout, "indexed %d files (%d skipped) — db: %s\n", indexed, skipped, dbPath)
	return nil
}

func currentHEAD(repoRoot string) string {
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return "init"
	}
	return strings.TrimSpace(string(out))
}
