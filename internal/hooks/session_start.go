// Package hooks contains Claude Code hook handlers.
// Hooks never block (exit code always 0) and inject systemMessage context via stdout JSON.
package hooks

import (
	"fmt"
	"strings"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
)

// SessionStartMessage returns the systemMessage to inject at session start.
// Single-root: compact summary. Multi-root: one line per root with remote and counts.
// Returns "" on error (hook silently skips).
func SessionStartMessage(q *graph.Querier) string {
	ctx, err := q.Context()
	if err != nil || ctx == nil {
		return ""
	}

	var sb strings.Builder

	if len(ctx.Roots) <= 1 {
		fmt.Fprintf(&sb, "[engrafo] %d simbolos indexados en %d lenguaje(s).\n",
			ctx.TotalNodes, len(ctx.Languages))
	} else {
		fmt.Fprintf(&sb, "[engrafo] Workspace: %d raiz(es) SEPARADAS (no monorepo).\n", len(ctx.Roots))
		for _, r := range ctx.Roots {
			remote := r.Remote
			if remote == "" {
				remote = "(sin remote)"
			}
			langs := strings.Join(r.Languages, ",")
			if langs == "" {
				langs = "-"
			}
			fmt.Fprintf(&sb, "- %s  %s  %s  %d simbolos  %s\n",
				r.Name, r.Path, remote, r.TotalNodes, langs)
		}
		sb.WriteString("Cada raiz tiene su propio control de versiones. " +
			"Para commit/push: cd a la ruta de la raiz y opera ahi.\n")
	}

	// Behavioral directives — injected every session to counteract CLAUDE.md drift.
	sb.WriteString("\n[engrafo] REGLAS ACTIVAS:\n")
	sb.WriteString("- Antes de editar un archivo: consulta cg_node para ver dependientes\n")
	sb.WriteString("- Al descubrir una decision arquitectonica: llama mem_save\n")
	sb.WriteString("- Al encontrar un bug y su causa raiz: llama mem_save\n")
	sb.WriteString("- Antes de que el contexto se compacte: llama mem_save por cada tema pendiente\n")
	sb.WriteString("- cg_deadcode para detectar codigo sin uso antes de refactors grandes")

	return sb.String()
}
