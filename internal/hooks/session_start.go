// Package hooks contains Claude Code hook handlers.
// Hooks never block (exit code always 0) and inject systemMessage context via stdout JSON.
package hooks

import (
	"fmt"
	"strings"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
)

// SessionStartMessage returns the systemMessage to inject at session start.
// Single-root: compact summary. Multi-root: una lÃ­nea por raÃ­z con remote y conteos.
// Returns "" on error (hook silently skips).
func SessionStartMessage(q *graph.Querier) string {
	ctx, err := q.Context()
	if err != nil || ctx == nil {
		return ""
	}

	if len(ctx.Roots) <= 1 {
		return fmt.Sprintf("[engrafo] project: %d symbols across %d language(s)",
			ctx.TotalNodes, len(ctx.Languages))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "[engrafo] Workspace: %d raÃ­z(es) SEPARADAS (no monorepo).\n", len(ctx.Roots))
	for _, r := range ctx.Roots {
		remote := r.Remote
		if remote == "" {
			remote = "(sin remote)"
		}
		langs := strings.Join(r.Languages, ",")
		if langs == "" {
			langs = "â€”"
		}
		fmt.Fprintf(&sb, "- %s  %s  %s  %d sÃ­mbolos  %s\n",
			r.Name, r.Path, remote, r.TotalNodes, langs)
	}
	sb.WriteString("Cada raÃ­z tiene su propio control de versiones. " +
		"Para commit/push: cd a la ruta de la raÃ­z y opera ahÃ­.")
	return sb.String()
}
