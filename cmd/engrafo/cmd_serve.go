package main

import (
	"fmt"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/mcp"
)

func cmdServe(cfg *config) error {
	dbPath, err := cfg.resolveDB()
	if err != nil {
		return err
	}

	s, err := graph.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db %s: %w", dbPath, err)
	}
	defer s.Close()

	return mcp.New(s).Serve()
}
