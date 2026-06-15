package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Jomruizgo/Engrafo/v2/internal/engram"
)

func cmdHooks(cfg *config, sub []string) error {
	if len(sub) == 0 {
		return fmt.Errorf("hooks: specify subcommand (install | uninstall)")
	}
	global := false
	args := sub[1:]
	for _, a := range args {
		if a == "--global" {
			global = true
		}
	}
	switch sub[0] {
	case "install":
		return hooksInstall(cfg, global)
	case "uninstall":
		return hooksUninstall(cfg, global)
	default:
		return fmt.Errorf("hooks: unknown subcommand %q", sub[0])
	}
}

// detectAgentDir returns (repoRoot, agentConfigDir).
// Walks up from CWD looking for .claude/ or .opencode/, defaulting to .claude/.
func detectAgentDir() (string, string) {
	cwd, _ := os.Getwd()
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".claude")); err == nil {
			return dir, filepath.Join(dir, ".claude")
		}
		if _, err := os.Stat(filepath.Join(dir, ".opencode")); err == nil {
			return dir, filepath.Join(dir, ".opencode")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd, filepath.Join(cwd, ".claude")
}

// projectRoot resolves the project root: workspace dir (manifest) > git root > cwd.
// Assumes cfg.resolveDB() has already run (it populates cfg.workspaceDir).
func projectRoot(cfg *config) string {
	if cfg.workspaceDir != "" {
		return cfg.workspaceDir
	}
	if gitRoot, err := findGitRoot(); err == nil {
		return gitRoot
	}
	cwd, _ := os.Getwd()
	return cwd
}

func hooksInstall(cfg *config, global bool) error {
	var agentDir string
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home dir: %w", err)
		}
		agentDir = filepath.Join(home, ".claude")
	} else {
		_, agentDir = detectAgentDir()
	}
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}

	// Ensure engram is installed. Failure is non-fatal.
	fmt.Fprintf(cfg.stdout, "checking engram...\n")
	if err := engram.EnsureCompatible(cfg.stdout); err != nil {
		fmt.Fprintf(cfg.stdout, "  [WARN] continuing without engram - cg_anchor unavailable\n")
	}

	// Write the MCP server config. .mcp.json is the project-scoped file that both
	// Claude Code CLI and the VSCode extension read (settings.json mcpServers is
	// Claude Desktop only).
	if err := registerMCPServers(cfg, global); err != nil {
		fmt.Fprintf(cfg.stdout, "  [WARN] MCP config failed: %v\n", err)
	}

	settingsPath := filepath.Join(agentDir, "settings.json")
	settings := readJSONFile(settingsPath)

	// Hook entries - replace the hooks map entirely for engrafo-managed events.
	// We preserve non-engrafo event keys.
	hooksCfg, _ := settings["hooks"].(map[string]any)
	if hooksCfg == nil {
		hooksCfg = map[string]any{}
	}

	hooksCfg["SessionStart"] = []any{
		map[string]any{"hooks": []any{
			map[string]any{"type": "command", "command": "engrafo hook session-start", "timeout": 15},
		}},
	}
	hooksCfg["PreToolUse"] = []any{
		map[string]any{"matcher": "Read", "hooks": []any{
			map[string]any{"type": "command", "command": "engrafo hook pre-read", "timeout": 3},
		}},
		map[string]any{"matcher": "Edit|Write|MultiEdit", "hooks": []any{
			map[string]any{"type": "command", "command": "engrafo hook pre-write", "timeout": 5},
		}},
	}
	hooksCfg["PostToolUse"] = []any{
		map[string]any{"matcher": "mcp__engram__mem_save", "hooks": []any{
			map[string]any{"type": "command", "command": "engrafo hook post-mem-save", "timeout": 10},
		}},
	}
	hooksCfg["PreCompact"] = []any{
		map[string]any{"hooks": []any{
			map[string]any{"type": "command", "command": "engrafo hook pre-compact", "timeout": 8},
		}},
	}
	settings["hooks"] = hooksCfg

	if err := writeJSONFile(settingsPath, settings); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	// Install git post-commit hook (only in project mode, not global)
	if !global {
		if gitRoot, err := findGitRoot(); err == nil {
			installGitPostCommitHook(gitRoot)
		}
	}

	fmt.Fprintf(cfg.stdout, "hooks installed: %s\n", settingsPath)
	return nil
}

// registerMCPServers writes .mcp.json at the project root with engrafo + engram.
// All values are resolved at install time so the config is deterministic and never
// depends on PATH lookups or CWD-based auto-detection at runtime:
//   - engrafo binary -> the currently running executable (os.Executable)
//   - graph db        -> the resolved -db path
//   - engram --project -> the workspace/repo name (one-brain namespacing)
func registerMCPServers(cfg *config, global bool) error {
	if global {
		return registerGlobalMCP(cfg)
	}

	dbPath, err := cfg.resolveDB()
	if err != nil {
		return fmt.Errorf("resolve db: %w", err)
	}

	root := projectRoot(cfg)
	projectName := filepath.Base(root)

	engrafoBin := "engrafo"
	if exe, eerr := os.Executable(); eerr == nil {
		engrafoBin = filepath.ToSlash(exe)
	}

	servers := map[string]any{
		"engrafo": map[string]any{
			"type":    "stdio",
			"command": engrafoBin,
			"args":    []any{"-db", filepath.ToSlash(dbPath), "serve"},
		},
	}

	// engram: bake in --project so its ambiguous .git auto-detection is bypassed.
	// One-brain namespacing: all workspace memory lives under the project name.
	if engramBin, lerr := exec.LookPath("engram"); lerr == nil {
		servers["engram"] = map[string]any{
			"type":    "stdio",
			"command": filepath.ToSlash(engramBin),
			"args":    []any{"mcp", "--project", projectName, "--tools=agent"},
		}
	} else {
		fmt.Fprintf(cfg.stdout, "  [WARN] engram no encontrado - omitido del .mcp.json\n")
	}

	mcpPath := filepath.Join(root, ".mcp.json")
	existing := readJSONFile(mcpPath)
	merged, _ := existing["mcpServers"].(map[string]any)
	if merged == nil {
		merged = map[string]any{}
	}
	for k, v := range servers {
		merged[k] = v
	}
	existing["mcpServers"] = merged

	if err := writeJSONFile(mcpPath, existing); err != nil {
		return fmt.Errorf("write .mcp.json: %w", err)
	}
	fmt.Fprintf(cfg.stdout, "MCP config: %s (project=%s)\n", mcpPath, projectName)
	return nil
}

// registerGlobalMCP registers engrafo at user scope via the claude CLI.
// Global scope has no single project, so -db and --project are auto-detected at runtime.
func registerGlobalMCP(cfg *config) error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude CLI not in PATH; run manually: claude mcp add engrafo -s user -- engrafo serve")
	}
	cmd := exec.Command("claude", "mcp", "add", "engrafo", "-s", "user", "--", "engrafo", "serve")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	return nil
}

func hooksUninstall(cfg *config, global bool) error {
	var agentDir string
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home dir: %w", err)
		}
		agentDir = filepath.Join(home, ".claude")
	} else {
		_, agentDir = detectAgentDir()
	}

	// Remove MCP servers from .mcp.json (project mode) or user scope (global).
	if global {
		if _, err := exec.LookPath("claude"); err == nil {
			for _, name := range []string{"engrafo", "engram"} {
				exec.Command("claude", "mcp", "remove", name, "-s", "user").Run() //nolint:errcheck
			}
		}
	} else {
		_, _ = cfg.resolveDB() // populate workspaceDir
		mcpPath := filepath.Join(projectRoot(cfg), ".mcp.json")
		mcp := readJSONFile(mcpPath)
		if servers, ok := mcp["mcpServers"].(map[string]any); ok {
			delete(servers, "engrafo")
			delete(servers, "engram")
			if len(servers) == 0 {
				delete(mcp, "mcpServers")
			}
			writeJSONFile(mcpPath, mcp) //nolint:errcheck
		}
	}

	settingsPath := filepath.Join(agentDir, "settings.json")
	settings := readJSONFile(settingsPath)

	// Remove hook events engrafo owns
	if hooksCfg, ok := settings["hooks"].(map[string]any); ok {
		delete(hooksCfg, "SessionStart")
		delete(hooksCfg, "PreToolUse")
		delete(hooksCfg, "PostToolUse")
		delete(hooksCfg, "PreCompact")
	}

	if err := writeJSONFile(settingsPath, settings); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	fmt.Fprintf(cfg.stdout, "hooks removed: %s\n", settingsPath)
	return nil
}

func installGitPostCommitHook(repoRoot string) {
	hookPath := filepath.Join(repoRoot, ".git", "hooks", "post-commit")
	content := "#!/bin/sh\nengrafo update 2>/dev/null || true\n"
	existing, err := os.ReadFile(hookPath)
	if err == nil && len(existing) > 0 {
		return // don't overwrite existing post-commit hook
	}
	os.WriteFile(hookPath, []byte(content), 0755)
}

func readJSONFile(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]any{}
	}
	return m
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
