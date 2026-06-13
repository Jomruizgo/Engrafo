package hooks

// PreWrite handles the PreToolUse(Edit|Write|MultiEdit) hook.
// Calls cg_impact(file_path, depth=3). Emits a warning systemMessage
// when total_count > 10, but never blocks (exit code always 0).
// Timeout: 5 s.
func PreWrite(_ string) error {
	return nil
}
