package main

import (
	"encoding/json"
	"fmt"
	"os"
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

	// Ensure engram is installed. Failure is non-fatal: the user can install
	// engram manually later; engrafo hooks still work without it.
	fmt.Fprintf(cfg.stdout, "checking engram...\n")
	if err := engram.EnsureCompatible(cfg.stdout); err != nil {
		fmt.Fprintf(cfg.stdout, "  [WARN] continuing without engram â€” cg_anchor unavailable\n")
	}

	settingsPath := filepath.Join(agentDir, "settings.json")
	settings := readJSONFile(settingsPath)

	// MCP server entries â€” engrafo + engram
	mcpServers, _ := settings["mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}
	mcpServers["engrafo"] = map[string]any{
		"command": "engrafo",
		"args":    []any{"serve"},
		"env":     map[string]any{},
	}
	mcpServers["engram"] = map[string]any{
		"command": "engram",
		"args":    []any{"mcp"},
		"env":     map[string]any{},
	}
	settings["mcpServers"] = mcpServers

	// Hook entries â€” replace the hooks map entirely for engrafo-managed events.
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
	settingsPath := filepath.Join(agentDir, "settings.json")
	settings := readJSONFile(settingsPath)

	// Remove MCP servers added by engrafo
	if mcpServers, ok := settings["mcpServers"].(map[string]any); ok {
		delete(mcpServers, "engrafo")
		delete(mcpServers, "engram")
	}

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
