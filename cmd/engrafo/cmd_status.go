package main

import (
	"fmt"
	"strings"

	"github.com/Jomruizgo/Engrafo/internal/graph"
)

func cmdStatus(cfg *config) error {
	dbPath, err := cfg.resolveDB()
	if err != nil {
		return err
	}

	s, err := graph.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db %s: %w", dbPath, err)
	}
	defer s.Close()

	q := graph.NewQuerier(s)
	ctx, err := q.Context()
	if err != nil {
		return fmt.Errorf("context: %w", err)
	}

	db := s.DB()
	var lastCommit, indexedAt, repoRoot string
	db.QueryRow(`SELECT value FROM index_meta WHERE key='last_commit_hash'`).Scan(&lastCommit)
	db.QueryRow(`SELECT value FROM index_meta WHERE key='indexed_at'`).Scan(&indexedAt)
	db.QueryRow(`SELECT value FROM index_meta WHERE key='repo_root'`).Scan(&repoRoot)

	if lastCommit == "" {
		lastCommit = "(none)"
	}
	if indexedAt == "" {
		indexedAt = "(unknown)"
	}

	fmt.Fprintf(cfg.stdout, "engrafo status\n")
	fmt.Fprintf(cfg.stdout, "  db:          %s\n", dbPath)
	fmt.Fprintf(cfg.stdout, "  nodes:       %d\n", ctx.TotalNodes)
	fmt.Fprintf(cfg.stdout, "  languages:   %s\n", strings.Join(ctx.Languages, ", "))
	fmt.Fprintf(cfg.stdout, "  last commit: %s\n", lastCommit)
	fmt.Fprintf(cfg.stdout, "  indexed at:  %s\n", indexedAt)
	if repoRoot != "" {
		fmt.Fprintf(cfg.stdout, "  repo root:   %s\n", repoRoot)
	}
	return nil
}
