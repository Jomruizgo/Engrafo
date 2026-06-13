package main

import (
	"encoding/json"
	"flag"
	"fmt"

	"github.com/Jomruizgo/Engrafo/internal/graph"
)

func cmdDeadcode(cfg *config, args []string) error {
	fs := flag.NewFlagSet("deadcode", flag.ContinueOnError)
	jsonOutput := fs.Bool("json", false, "output JSON instead of human-readable text")
	thresholdDays := fs.Int("threshold-days", 0, "only include nodes inactive for more than N days")
	fs.SetOutput(cfg.stdout)
	if err := fs.Parse(args); err != nil {
		return err
	}

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
	result, err := q.Deadcode(*thresholdDays)
	if err != nil {
		return fmt.Errorf("deadcode scan: %w", err)
	}

	if *jsonOutput {
		enc := json.NewEncoder(cfg.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Fprintf(cfg.stdout, "orphans (%d — never referenced):\n", len(result.Orphans))
	for _, o := range result.Orphans {
		fmt.Fprintf(cfg.stdout, "  %-30s  %s  %s\n", o.Symbol, o.Kind, o.FilePath)
	}
	if len(result.Orphans) == 0 {
		fmt.Fprintf(cfg.stdout, "  (none)\n")
	}

	fmt.Fprintf(cfg.stdout, "\nabandoned (%d — once referenced, now not):\n", len(result.Abandoned))
	for _, a := range result.Abandoned {
		fmt.Fprintf(cfg.stdout, "  %-30s  %s  %s  peak=%d  days=%.0f\n",
			a.Symbol, a.Kind, a.FilePath, a.PeakIncomingEdges, a.DaysSinceAbandoned)
	}
	if len(result.Abandoned) == 0 {
		fmt.Fprintf(cfg.stdout, "  (none)\n")
	}
	return nil
}
