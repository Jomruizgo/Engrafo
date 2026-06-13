// Package mcp implements the MCP server (stdio transport).
// Full implementation: feature/mcp-tools.
package mcp

import "github.com/Jomruizgo/Engrafo/internal/graph"

// Server is the engrafo MCP server.
// It exposes exactly 7 tools; see tools.go.
type Server struct {
	store *graph.Store
}

// New creates a new Server backed by the given Store.
func New(s *graph.Store) *Server {
	return &Server{store: s}
}

// Serve starts the MCP server on stdio and blocks until the process exits.
func (s *Server) Serve() error {
	return nil
}
