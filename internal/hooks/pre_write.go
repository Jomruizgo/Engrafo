package hooks

import "github.com/Jomruizgo/Engrafo/internal/graph"

// PreWriteMessage returns the systemMessage to inject before an Edit/Write tool call.
// Always returns a message (informative or warning if impact > 10 nodes).
func PreWriteMessage(_ *graph.Querier, _ string, _ int) string {
	return "" // BLOQUEANTE: stub
}
