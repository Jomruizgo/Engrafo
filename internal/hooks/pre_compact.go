package hooks

import (
	"fmt"
	"strings"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
)

// PreCompactMessage returns the systemMessage to inject before context compaction.
// This is the critical enforcement point: the agent is about to lose conversation
// context, so we remind it to persist important decisions to engram before that happens.
func PreCompactMessage(q *graph.Querier) string {
	var sb strings.Builder

	sb.WriteString("[engrafo] COMPACTACION INMINENTE — antes de compactar, llama mem_save para:\n")
	sb.WriteString("  1. Decisiones arquitectonicas tomadas en esta sesion\n")
	sb.WriteString("  2. Bugs encontrados y sus causas raiz\n")
	sb.WriteString("  3. Estado de features en progreso (que falta, que esta bloqueado)\n")
	sb.WriteString("  4. Patrones o convenciones descubiertas del codebase\n")
	sb.WriteString("Usa una llamada mem_save por tema. Sin mem_save, este contexto se pierde.")

	if q != nil {
		if ctx, err := q.Context(); err == nil && ctx != nil {
			fmt.Fprintf(&sb, "\n[engrafo] grafo activo: %d simbolos en %d raiz(es)",
				ctx.TotalNodes, len(ctx.Roots))
		}
	}

	return sb.String()
}
