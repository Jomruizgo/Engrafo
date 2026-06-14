package main

import (
	"fmt"
	"os"

	"github.com/Jomruizgo/Engrafo/v2/internal/engram"
	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
	"github.com/Jomruizgo/Engrafo/v2/internal/version"
)

func cmdDoctor(cfg *config) error {
	dbPath, err := cfg.resolveDB()
	if err != nil {
		return fmt.Errorf("doctor: cannot resolve db path: %w", err)
	}

	allPass := true
	check := func(ok bool, label, hint string) {
		if ok {
			fmt.Fprintf(cfg.stdout, "  [OK]   %s\n", label)
		} else {
			fmt.Fprintf(cfg.stdout, "  [FAIL] %s\n", label)
			if hint != "" {
				fmt.Fprintf(cfg.stdout, "         %s\n", hint)
			}
			allPass = false
		}
	}

	fmt.Fprintf(cfg.stdout, "engrafo doctor (v%s)\n", version.Current)

	// Check 1: db file exists
	_, statErr := os.Stat(dbPath)
	check(statErr == nil,
		"db exists: "+dbPath,
		"run: engrafo init")

	// Check 2: db readable (only when file exists)
	if statErr == nil {
		s, openErr := graph.Open(dbPath)
		if openErr == nil {
			s.Close()
		}
		check(openErr == nil, "db readable", "db may be corrupt â€” re-run: engrafo init")
	}

	// Check 3: engram
	es := engram.Detect()
	switch {
	case !es.Found:
		fmt.Fprintf(cfg.stdout, "  [FAIL] engram â€” not found\n")
		fmt.Fprintf(cfg.stdout, "         cg_anchor and observation features unavailable\n")
		fmt.Fprintf(cfg.stdout, "         run: engrafo hooks install  (auto-installs engram %s)\n",
			version.EngramCompatible)
		allPass = false
	case es.Newer:
		fmt.Fprintf(cfg.stdout, "  [WARN] engram v%s â€” newer than tested (%s); compatibility not guaranteed\n",
			es.Version, version.EngramCompatible)
	default:
		fmt.Fprintf(cfg.stdout, "  [OK]   engram v%s\n", es.Version)
	}

	if !allPass {
		return fmt.Errorf("doctor: one or more checks failed")
	}
	return nil
}
