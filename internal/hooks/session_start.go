// Package hooks contains Claude Code hook handlers.
// Hooks never block (exit code always 0) and inject systemMessage context via stdout JSON.
package hooks

import (
	"fmt"

	"github.com/Jomruizgo/Engrafo/internal/graph"
)

// SessionStartMessage returns the systemMessage to inject at session start.
// Calls Context and formats a project summary. Returns "" on error (hook silently skips).
func SessionStartMessage(q *graph.Querier) string {
	ctx, err := q.Context()
	if err != nil || ctx == nil {
		return ""
	}
	return fmt.Sprintf("[engrafo] project: %d symbols across %d language(s)", ctx.TotalNodes, len(ctx.Languages))
}
