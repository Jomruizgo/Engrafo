// Package mcp implements the MCP server (stdio transport).
package mcp

import (
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Jomruizgo/Engrafo/internal/graph"
)

// Server is the engrafo MCP server exposing exactly 7 tools.
type Server struct {
	handlers *Handlers
	mcp      *server.MCPServer
}

// New creates a new Server backed by the given Store and registers all 7 tools.
func New(s *graph.Store) *Server {
	h := NewHandlers(s)
	srv := &Server{handlers: h}
	srv.mcp = srv.buildMCP()
	return srv
}

// ToolCount returns the number of registered MCP tools.
func (s *Server) ToolCount() int {
	return len(s.mcp.ListTools())
}

// Serve starts the MCP server on stdio and blocks until the process exits.
func (s *Server) Serve() error {
	return server.ServeStdio(s.mcp)
}

// buildMCP creates and configures the underlying MCPServer with all 7 tools.
func (s *Server) buildMCP() *server.MCPServer {
	mcpSrv := server.NewMCPServer(
		"engrafo",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	mcpSrv.AddTool(
		mcplib.NewTool(ToolCGContext,
			mcplib.WithDescription("Returns the project-level graph summary: languages, node counts, top referenced symbols."),
		),
		s.handlers.CGContext,
	)

	mcpSrv.AddTool(
		mcplib.NewTool(ToolCGNode,
			mcplib.WithDescription("Returns full details for a symbol: location, edges (depends_on, used_by), engram anchors."),
			mcplib.WithString("symbol", mcplib.Required(), mcplib.Description("Symbol name to look up")),
			mcplib.WithString("kind", mcplib.Description("Optional kind filter: function, class, method, interface, package")),
			mcplib.WithBoolean("include_invalidated", mcplib.Description("If true, include historical invalidated edges")),
		),
		s.handlers.CGNode,
	)

	mcpSrv.AddTool(
		mcplib.NewTool(ToolCGDependents,
			mcplib.WithDescription("Returns all nodes that depend on the given file or symbol (blast radius upward)."),
			mcplib.WithString("file_path", mcplib.Description("File path to look up dependents for")),
			mcplib.WithString("symbol", mcplib.Description("Symbol name to look up dependents for")),
		),
		s.handlers.CGDependents,
	)

	mcpSrv.AddTool(
		mcplib.NewTool(ToolCGDependencies,
			mcplib.WithDescription("Returns everything the given file or symbol depends on (dependencies downward)."),
			mcplib.WithString("file_path", mcplib.Description("File path to look up dependencies for")),
			mcplib.WithString("symbol", mcplib.Description("Symbol name to look up dependencies for")),
		),
		s.handlers.CGDependencies,
	)

	mcpSrv.AddTool(
		mcplib.NewTool(ToolCGImpact,
			mcplib.WithDescription("Computes transitive blast radius of modifying a file up to depth hops."),
			mcplib.WithString("file_path", mcplib.Required(), mcplib.Description("File to compute impact for")),
			mcplib.WithNumber("depth", mcplib.Description("Traversal depth (default: 3)")),
		),
		s.handlers.CGImpact,
	)

	mcpSrv.AddTool(
		mcplib.NewTool(ToolCGSearch,
			mcplib.WithDescription("FTS5 search over symbol names and signatures."),
			mcplib.WithString("query", mcplib.Required(), mcplib.Description("Search query")),
			mcplib.WithNumber("limit", mcplib.Description("Max results (default: 10)")),
		),
		s.handlers.CGSearch,
	)

	mcpSrv.AddTool(
		mcplib.NewTool(ToolCGAnchor,
			mcplib.WithDescription("Links an engram observation to one or more graph nodes by symbol name."),
			mcplib.WithString("engram_obs_id", mcplib.Required(), mcplib.Description("Engram observation UUID")),
			mcplib.WithArray("symbols", mcplib.Required(), mcplib.Description("Symbol names to anchor the observation to")),
		),
		s.handlers.CGAnchor,
	)

	mcpSrv.AddTool(
		mcplib.NewTool(ToolCGDeadcode,
			mcplib.WithDescription("Scans for dead code: orphan nodes (never referenced) and abandoned nodes (once referenced, no longer)."),
			mcplib.WithNumber("threshold_days", mcplib.Description("Only include nodes inactive for more than N days (default: 0 = all)")),
		),
		s.handlers.CGDeadcode,
	)

	mcpSrv.AddTool(
		mcplib.NewTool(ToolCGHistory,
			mcplib.WithDescription("Returns the chronological edge timeline for a symbol: when dependencies appeared and disappeared, with associated engram anchors."),
			mcplib.WithString("symbol", mcplib.Required(), mcplib.Description("Symbol name to inspect")),
			mcplib.WithString("kind", mcplib.Description("Optional kind filter: function, class, method, interface")),
		),
		s.handlers.CGHistory,
	)

	return mcpSrv
}
