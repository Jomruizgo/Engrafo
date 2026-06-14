package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Jomruizgo/Engrafo/internal/workspace"
)

// config holds shared state passed to all subcommands.
type config struct {
	dbPath        string
	manifestPath  string // ruta al engrafo.json encontrado; "" si no hay manifest
	workspaceDir  string // directorio del manifest; "" si no hay manifest
	stdin         io.Reader
	stdout        io.Writer
}

// resolveDB returns the db path according to the precedence defined in §3 of plan-v2.0:
//  1. --db flag (explicit override)
//  2. engrafo.json manifest found walking up from CWD → <wsdir>/.engrafo/graph.db
//  3. .git found walking up from CWD → <gitroot>/.engrafo/graph.db
//  4. CWD fallback → <cwd>/.engrafo/graph.db
func (cfg *config) resolveDB() (string, error) {
	if cfg.dbPath != "" {
		return cfg.dbPath, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return cfg.resolveDBFrom(cwd)
}

// resolveDBFrom implementa la precedencia de resolveDB con un directorio de inicio configurable.
// Exportado internamente para tests sin cambiar el CWD del proceso.
func (cfg *config) resolveDBFrom(startDir string) (string, error) {
	if cfg.dbPath != "" {
		return cfg.dbPath, nil
	}

	// Paso 2: buscar manifest.
	if mp, wsDir, ok := workspace.FindManifest(startDir); ok {
		cfg.manifestPath = mp
		cfg.workspaceDir = wsDir
		return filepath.Join(wsDir, ".engrafo", "graph.db"), nil
	}

	// Paso 3: buscar .git.
	if gitRoot, ok := workspace.FindGitRoot(startDir); ok {
		return filepath.Join(gitRoot, ".engrafo", "graph.db"), nil
	}

	// Paso 4: CWD fallback.
	return filepath.Join(startDir, ".engrafo", "graph.db"), nil
}

// findGitRoot walks parent directories until it finds a .git directory.
// Kept for backward compat with cmd_update.go; delegates to workspace package.
func findGitRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if root, ok := workspace.FindGitRoot(cwd); ok {
		return root, nil
	}
	return "", errors.New("not inside a git repository")
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
	case "deadcode":
		return cmdDeadcode(cfg, rest[1:])
	case "ui":
		return cmdUI(cfg, rest[1:])
	case "workspace":
		return cmdWorkspace(cfg, rest[1:])
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
  deadcode             list dead-code candidates (orphans + abandoned)
  ui                   start graph browser on localhost:8080
  workspace add        add a root to the workspace manifest
  workspace list       list registered roots
  workspace remove     remove a root from the workspace

Flags:
  --db <path>          override graph.db path (default: .engrafo/graph.db)
`)
}
