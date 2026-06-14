package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
)

func cmdUpdate(cfg *config) error {
	dbPath, err := cfg.resolveDB()
	if err != nil {
		return err
	}

	s, err := graph.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer s.Close()

	db := s.DB()
	var lastCommit string
	db.QueryRow(`SELECT value FROM index_meta WHERE key='last_commit_hash'`).Scan(&lastCommit)
	if lastCommit == "" {
		return fmt.Errorf("no previous index found — run 'engrafo init' first")
	}

	repoRoot, err := findGitRoot()
	if err != nil {
		return err
	}

	headOut, err := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	currentCommit := strings.TrimSpace(string(headOut))

	if currentCommit == lastCommit {
		fmt.Fprintf(cfg.stdout, "already up to date at %s\n", currentCommit[:12])
		return nil
	}

	// Get or create root for this repo.
	rootID, err := resolveUpdateRoot(s, repoRoot)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}

	revID, err := s.CreateRevision(rootID, "git", currentCommit)
	if err != nil {
		return fmt.Errorf("create revision: %w", err)
	}

	diffOut, err := exec.Command("git", "-C", repoRoot, "diff", "--name-only",
		lastCommit+".."+currentCommit).Output()
	if err != nil {
		return fmt.Errorf("git diff: %w", err)
	}

	changed := strings.Split(strings.TrimSpace(string(diffOut)), "\n")
	p := newParser()
	b := graph.NewBuilder(s)

	var updated, skipped int
	for _, relPath := range changed {
		if relPath == "" || parser.Detect(relPath) == "" {
			continue
		}
		absPath := filepath.Join(repoRoot, relPath)
		result, parseErr := p.ParseFile(absPath)
		if parseErr != nil {
			skipped++
			continue
		}
		norm := filepath.ToSlash(relPath)
		for i := range result.Nodes {
			result.Nodes[i].FilePath = norm
		}
		if upsertErr := b.UpsertFile(rootID, revID, "", result); upsertErr != nil {
			return fmt.Errorf("upsert %s: %w", relPath, upsertErr)
		}
		updated++
	}

	db.Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('last_commit_hash',?)`, currentCommit)
	db.Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('indexed_at',datetime('now'))`)
	s.SetRootIndexed(rootID, currentCommit)

	fmt.Fprintf(cfg.stdout, "updated %d files (%d skipped) — now at %s\n", updated, skipped, currentCommit[:12])
	return nil
}

// resolveUpdateRoot returns the root ID for the given repo path.
// Uses the first existing root if available (single-repo mode), else creates one.
func resolveUpdateRoot(s *graph.Store, repoRoot string) (int64, error) {
	roots, err := s.AllRoots()
	if err != nil {
		return 0, err
	}
	if len(roots) > 0 {
		return roots[0].ID, nil
	}
	return s.UpsertRoot(graph.ResolvedRoot{
		Name:    filepath.Base(repoRoot),
		RelPath: ".",
		AbsRoot: repoRoot,
		VCS:     "git",
	})
}
