package hooks

import (
	"fmt"

	"github.com/Jomruizgo/Engrafo/internal/graph"
)

// PreWriteMessage returns the systemMessage to inject before an Edit/Write tool call.
// Always returns a message — warning when impact > 10 nodes, informative otherwise.
func PreWriteMessage(q *graph.Querier, filePath string, depth int) string {
	impact, err := q.Impact(filePath, depth)
	if err != nil {
		return fmt.Sprintf("[engrafo] could not compute impact for %s", filePath)
	}
	if len(impact) > 10 {
		return fmt.Sprintf("[engrafo] WARNING: modifying %s may impact %d files — review blast radius", filePath, len(impact))
	}
	return fmt.Sprintf("[engrafo] modifying %s may impact %d file(s)", filePath, len(impact))
}
