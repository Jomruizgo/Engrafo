package hooks

// PreRead handles the PreToolUse(Read) hook.
// Calls cg_dependents for the file being read and injects a systemMessage
// only when the file has ≥1 dependent in the graph.
// Timeout: 3 s. Exit code: always 0.
func PreRead(_ string) error {
	return nil
}
