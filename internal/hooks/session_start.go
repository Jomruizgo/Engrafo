// Package hooks contains Claude Code hook handlers.
// Hooks never block (exit code always 0) and inject systemMessage context via stdout JSON.
package hooks

import "github.com/Jomruizgo/Engrafo/internal/graph"

// SessionStartMessage returns the systemMessage to inject at session start.
// Calls cg_context and formats a project summary.
// Returns "" on error (hook should silently continue).
func SessionStartMessage(_ *graph.Querier) string {
	return "" // BLOQUEANTE: stub
}
