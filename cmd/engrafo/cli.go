package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// config holds shared state passed to all subcommands.
type config struct {
	dbPath string
	stdin  io.Reader
	stdout io.Writer
}

// resolveDB returns the db path from the flag, or auto-detects .engrafo/graph.db
// relative to the git root (or CWD when not in a git repo).
func (cfg *config) resolveDB() (string, error) {
	if cfg.dbPath != "" {
		return cfg.dbPath, nil
	}
	root, err := findGitRoot()
	if err != nil {
		root, _ = os.Getwd()
	}
	return filepath.Join(root, ".engrafo", "graph.db"), nil
}

// findGitRoot walks parent directories until it finds a .git directory.
func findGitRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("not inside a git repository")
		}
		dir = parent
	}
}

// runWith is the testable entry point for the CLI.
func runWith(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("engrafo", flag.ContinueOnError)
	fs.SetOutput(stdout)
	dbFlag := fs.String("db", "", "path to graph.db (overrides auto-detection)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 {
		printUsage(stdout)
		return nil
	}

	cfg := &config{dbPath: *dbFlag, stdin: stdin, stdout: stdout}

	switch rest[0] {
	case "serve":
		return cmdServe(cfg)
	case "init":
		return cmdInit(cfg, rest[1:])
	case "update":
		return cmdUpdate(cfg)
	case "status":
		return cmdStatus(cfg)
	case "query":
		return cmdQuery(cfg, rest[1:])
	case "hooks":
		return cmdHooks(cfg, rest[1:])
	case "doctor":
		return cmdDoctor(cfg)
	case "hook":
		return cmdHook(cfg, rest[1:])
	default:
		return fmt.Errorf("unknown command %q — run engrafo --help", rest[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `engrafo — structural code graph for coding agents

Usage:
  engrafo [--db <path>] <command> [args]

Commands:
  serve                start MCP server (stdio)
  init [root]          index repo from scratch
  update               update index since last commit
  status               show index statistics
  query <symbol>       query a symbol
  hooks install        install Claude Code hooks
  hooks uninstall      remove Claude Code hooks
  doctor               verify installation
  hook session-start   session-start hook handler (reads stdin JSON)
  hook pre-read        pre-read hook handler
  hook pre-write       pre-write hook handler

Flags:
  --db <path>          override graph.db path (default: .engrafo/graph.db)
`)
}
