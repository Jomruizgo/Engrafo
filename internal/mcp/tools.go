package mcp

import (
	"context"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/Jomruizgo/Engrafo/internal/graph"
)

// Tool names exposed by the MCP server. PRD mandates exactly these 7.
const (
	ToolCGContext      = "cg_context"
	ToolCGNode         = "cg_node"
	ToolCGDependents   = "cg_dependents"
	ToolCGDependencies = "cg_dependencies"
	ToolCGImpact       = "cg_impact"
	ToolCGSearch       = "cg_search"
	ToolCGAnchor       = "cg_anchor"
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

func (h *Handlers) CGContext(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return nil, nil // BLOQUEANTE: stub
}

func (h *Handlers) CGNode(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return nil, nil // BLOQUEANTE: stub
}

func (h *Handlers) CGDependents(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return nil, nil // BLOQUEANTE: stub
}

func (h *Handlers) CGDependencies(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return nil, nil // BLOQUEANTE: stub
}

func (h *Handlers) CGImpact(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return nil, nil // BLOQUEANTE: stub
}

func (h *Handlers) CGSearch(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return nil, nil // BLOQUEANTE: stub
}

func (h *Handlers) CGAnchor(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return nil, nil // BLOQUEANTE: stub
}
