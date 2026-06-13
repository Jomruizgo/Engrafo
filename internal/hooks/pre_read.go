package hooks

import "github.com/Jomruizgo/Engrafo/internal/graph"

// PreReadMessage returns the systemMessage to inject before a Read tool call.
// Returns "" when the file has no dependents (no injection needed).
func PreReadMessage(_ *graph.Querier, _ string) string {
	return "" // BLOQUEANTE: stub
}
