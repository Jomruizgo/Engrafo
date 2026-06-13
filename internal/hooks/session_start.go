// Package hooks contains Claude Code hook handlers.
// Full implementation: feature/hooks.
package hooks

// SessionStart handles the SessionStart hook.
// Calls cg_context and injects a systemMessage with the project map.
// Exit code is always 0 — never blocks session start.
func SessionStart() error {
	return nil
}
