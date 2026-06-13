package hooks

// BuildOutput formats the hook stdout JSON response.
// Claude Code reads this JSON; systemMessage is injected into the agent context.
// Empty message → no systemMessage key (no injection).
func BuildOutput(_ string) string {
	return "{}" // BLOQUEANTE: stub
}
