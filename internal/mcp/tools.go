package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/Jomruizgo/Engrafo/internal/graph"
)

// Tool names exposed by the MCP server.
// 7 tools in v1.0 + cg_deadcode in v1.1.
const (
	ToolCGContext      = "cg_context"
	ToolCGNode         = "cg_node"
	ToolCGDependents   = "cg_dependents"
	ToolCGDependencies = "cg_dependencies"
	ToolCGImpact       = "cg_impact"
	ToolCGSearch       = "cg_search"
	ToolCGAnchor       = "cg_anchor"
	ToolCGDeadcode     = "cg_deadcode"
)

// Handlers holds the dependencies used by all tool handlers.
type Handlers struct {
	store   *graph.Store
	querier *graph.Querier
	builder *graph.Builder
}

// NewHandlers creates a Handlers backed by the given Store.
func NewHandlers(s *graph.Store) *Handlers {
	return &Handlers{
		store:   s,
		querier: graph.NewQuerier(s),
		builder: graph.NewBuilder(s),
	}
}

// jsonResult marshals v to JSON and wraps it in a TextContent result.
func jsonResult(v any) (*mcplib.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return mcplib.NewToolResultText(string(b)), nil
}

// errResult returns an error-flagged tool result (not a protocol error).
func errResult(msg string) *mcplib.CallToolResult {
	r := mcplib.NewToolResultText(fmt.Sprintf(`{"error":%q}`, msg))
	r.IsError = true
	return r
}

// CGContext implements cg_context: returns project-level summary.
func (h *Handlers) CGContext(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	ctx, err := h.querier.Context()
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(map[string]any{
		"languages": ctx.Languages,
		"stats":     ctx.NodeCounts,
		"top_nodes": ctx.TopNodes,
		"total":     ctx.TotalNodes,
	})
}

// CGNode implements cg_node: returns full details for a single symbol.
func (h *Handlers) CGNode(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()
	symbol, _ := args["symbol"].(string)
	if symbol == "" {
		return errResult("symbol is required"), nil
	}
	kind, _ := args["kind"].(string)
	includeInvalidated, _ := args["include_invalidated"].(bool)

	info, err := h.querier.NodeInfo(symbol, kind, includeInvalidated)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(map[string]any{
		"node":                    info.Node,
		"depends_on":              info.DependsOn,
		"used_by":                 info.UsedBy,
		"anchored_observations":   info.AnchoredObsIDs,
		"historical_edges":        info.HistoricalEdges,
	})
}

// CGDependents implements cg_dependents: nodes that depend on a file or symbol.
func (h *Handlers) CGDependents(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()
	filePath, _ := args["file_path"].(string)
	symbol, _ := args["symbol"].(string)

	target := filePath
	if target == "" {
		target = symbol
	}
	if target == "" {
		return errResult("file_path or symbol is required"), nil
	}

	deps, err := h.querier.Dependents(target)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(map[string]any{"dependents": deps})
}

// CGDependencies implements cg_dependencies: what a file or symbol depends on.
func (h *Handlers) CGDependencies(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()
	filePath, _ := args["file_path"].(string)
	symbol, _ := args["symbol"].(string)

	target := filePath
	if target == "" {
		target = symbol
	}
	if target == "" {
		return errResult("file_path or symbol is required"), nil
	}

	deps, err := h.querier.Dependencies(target)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(map[string]any{"dependencies": deps})
}

// CGImpact implements cg_impact: transitive blast radius of modifying a file.
func (h *Handlers) CGImpact(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return errResult("file_path is required"), nil
	}
	depth := 3
	if d, ok := args["depth"].(float64); ok && d > 0 {
		depth = int(d)
	}

	affected, err := h.querier.Impact(filePath, depth)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(map[string]any{
		"affected":    affected,
		"total_count": len(affected),
	})
}

// CGSearch implements cg_search: FTS5 symbol search.
func (h *Handlers) CGSearch(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()
	query, _ := args["query"].(string)
	if query == "" {
		return errResult("query is required"), nil
	}
	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	results, err := h.querier.Search(query, limit)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(map[string]any{"results": results})
}

// CGAnchor implements cg_anchor: links an engram observation to graph nodes.
func (h *Handlers) CGAnchor(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()
	obsID, _ := args["engram_obs_id"].(string)
	if obsID == "" {
		return errResult("engram_obs_id is required"), nil
	}

	var symbols []string
	if raw, ok := args["symbols"].([]any); ok {
		for _, s := range raw {
			if sym, ok := s.(string); ok {
				symbols = append(symbols, sym)
			}
		}
	}

	count, err := h.store.AnchorObservations(obsID, symbols)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(map[string]any{"anchored": count})
}

// CGDeadcode implements the cg_deadcode MCP tool (v1.1).
// Returns orphan nodes (never referenced) and abandoned nodes (once referenced, now not).
func (h *Handlers) CGDeadcode(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()
	thresholdDays := 0
	if v, ok := args["threshold_days"].(float64); ok {
		thresholdDays = int(v)
	}

	result, err := h.querier.Deadcode(thresholdDays)
	if err != nil {
		return errResult(fmt.Sprintf("deadcode scan: %v", err)), nil
	}

	orphans := make([]map[string]any, 0, len(result.Orphans))
	for _, o := range result.Orphans {
		orphans = append(orphans, map[string]any{
			"symbol":    o.Symbol,
			"kind":      o.Kind,
			"file_path": o.FilePath,
			"language":  o.Language,
		})
	}

	abandoned := make([]map[string]any, 0, len(result.Abandoned))
	for _, a := range result.Abandoned {
		abandoned = append(abandoned, map[string]any{
			"symbol":               a.Symbol,
			"kind":                 a.Kind,
			"file_path":            a.FilePath,
			"language":             a.Language,
			"peak_incoming_edges":  a.PeakIncomingEdges,
			"days_since_abandoned": a.DaysSinceAbandoned,
		})
	}

	return jsonResult(map[string]any{
		"orphans":   orphans,
		"abandoned": abandoned,
	})
}
