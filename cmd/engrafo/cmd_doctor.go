package main

import (
	"fmt"
	"os"

	"github.com/Jomruizgo/Engrafo/internal/graph"
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

	fmt.Fprintf(cfg.stdout, "engrafo doctor\n")

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
		check(openErr == nil, "db readable", "db may be corrupt — re-run: engrafo init")
	}

	if !allPass {
		return fmt.Errorf("doctor: one or more checks failed")
	}
	return nil
}
