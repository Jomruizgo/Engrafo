package mcp

// Tool names exposed by the MCP server.
// PRD mandates exactly these 7 — no more.
const (
	ToolCGContext      = "cg_context"
	ToolCGNode         = "cg_node"
	ToolCGDependents   = "cg_dependents"
	ToolCGDependencies = "cg_dependencies"
	ToolCGImpact       = "cg_impact"
	ToolCGSearch       = "cg_search"
	ToolCGAnchor       = "cg_anchor"
)
