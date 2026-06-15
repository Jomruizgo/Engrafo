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

	// Register MCP servers with Claude Code via `claude mcp add`.
	// Claude Code reads from ~/.claude.json (user scope) or .mcp.json (project scope),
	// NOT from settings.json mcpServers — that key is for Claude Desktop only.
	if err := registerMCPServers(cfg, global); err != nil {
		// Non-fatal: claude CLI might not be in PATH, user can register manually.
		fmt.Fprintf(cfg.stdout, "  [WARN] claude mcp add failed: %v\n", err)
		fmt.Fprintf(cfg.stdout, "  Run manually: claude mcp add engrafo -- engrafo serve\n")
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

// registerMCPServers calls `claude mcp add` to register engrafo with Claude Code CLI.
// Global mode uses user scope (~/.claude.json); project mode uses local scope (.claude/settings.local.json).
func registerMCPServers(cfg *config, global bool) error {
	scope := "local"
	if global {
		scope = "user"
	}

	// Build engrafo args: include explicit -db path in project mode.
	mcpArgs := []string{"serve"}
	if !global {
		if dbPath, err := cfg.resolveDB(); err == nil {
			mcpArgs = []string{"-db", filepath.ToSlash(dbPath), "serve"}
		}
	}

	// claude mcp add engrafo -s <scope> -- engrafo [args...]
	args := append([]string{"mcp", "add", "engrafo", "-s", scope, "--"}, append([]string{"engrafo"}, mcpArgs...)...)
	cmd := exec.Command("claude", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}

	// Register engram only if the binary exists.
	if _, err := exec.LookPath("engram"); err == nil {
		engramArgs := []string{"mcp", "add", "engram", "-s", scope, "--", "engram", "mcp"}
		engramCmd := exec.Command("claude", engramArgs...)
		if out, err := engramCmd.CombinedOutput(); err != nil {
			fmt.Fprintf(cfg.stdout, "  [WARN] engram mcp add failed: %s\n", out)
		}
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

	// Unregister MCP servers from Claude Code.
	scope := "local"
	if global {
		scope = "user"
	}
	for _, name := range []string{"engrafo", "engram"} {
		cmd := exec.Command("claude", "mcp", "remove", name, "-s", scope)
		cmd.CombinedOutput() //nolint:errcheck - best effort
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
