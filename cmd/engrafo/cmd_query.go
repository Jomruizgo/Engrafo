package main

import (
	"fmt"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
)

func cmdQuery(cfg *config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("query: requires <symbol>")
	}
	symbol := args[0]

	dbPath, err := cfg.resolveDB()
	if err != nil {
		return err
	}

	s, err := graph.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer s.Close()

	q := graph.NewQuerier(s)
	result, err := q.NodeInfo(symbol, "", false, "")
	if err != nil {
		return fmt.Errorf("query %q: %w", symbol, err)
	}

	n := result.Node
	fmt.Fprintf(cfg.stdout, "symbol:   %s\n", n.Symbol)
	fmt.Fprintf(cfg.stdout, "kind:     %s\n", n.Kind)
	fmt.Fprintf(cfg.stdout, "file:     %s:%d-%d\n", n.FilePath, n.LineStart, n.LineEnd)
	fmt.Fprintf(cfg.stdout, "language: %s\n", n.Language)

	if len(result.DependsOn) > 0 {
		fmt.Fprintf(cfg.stdout, "depends_on:\n")
		for _, d := range result.DependsOn {
			fmt.Fprintf(cfg.stdout, "  %s (%s) via %s\n", d.Symbol, d.Kind, d.EdgeKind)
		}
	}
	if len(result.UsedBy) > 0 {
		fmt.Fprintf(cfg.stdout, "used_by:\n")
		for _, d := range result.UsedBy {
			fmt.Fprintf(cfg.stdout, "  %s (%s) via %s\n", d.Symbol, d.Kind, d.EdgeKind)
		}
	}
	if len(result.AnchoredObsIDs) > 0 {
		fmt.Fprintf(cfg.stdout, "anchored_observations:\n")
		for _, id := range result.AnchoredObsIDs {
			fmt.Fprintf(cfg.stdout, "  %s\n", id)
		}
	}
	return nil
}
