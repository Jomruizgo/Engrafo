package hooks

import (
	"fmt"

	"github.com/Jomruizgo/Engrafo/internal/graph"
)

// PreReadMessage returns the systemMessage to inject before a Read tool call.
// Returns "" when the file has no dependents (no injection needed).
func PreReadMessage(q *graph.Querier, filePath string) string {
	deps, err := q.Dependents(filePath)
	if err != nil || len(deps) == 0 {
		return ""
	}
	return fmt.Sprintf("[engrafo] %s has %d dependent(s) — edits may break callers", filePath, len(deps))
}
