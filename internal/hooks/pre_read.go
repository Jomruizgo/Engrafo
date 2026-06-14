package hooks

import (
	"fmt"

	"github.com/Jomruizgo/Engrafo/internal/graph"
)

// PreReadMessage returns the systemMessage to inject before a Read tool call.
// Returns "" when the file has no dependents (no injection needed).
// rootName filters to a specific root; "" queries all roots.
func PreReadMessage(q *graph.Querier, filePath, rootName string) string {
	deps, err := q.Dependents(filePath, rootName)
	if err != nil || len(deps) == 0 {
		return ""
	}
	if rootName != "" {
		return fmt.Sprintf("[engrafo] %s en %s: %d dependiente(s) — editar puede romper llamadores",
			filePath, rootName, len(deps))
	}
	return fmt.Sprintf("[engrafo] %s has %d dependent(s) — edits may break callers", filePath, len(deps))
}
