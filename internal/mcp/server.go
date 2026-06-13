// Package mcp implements the MCP server (stdio transport).
package mcp

import (
	"github.com/Jomruizgo/Engrafo/internal/graph"
)

// Server is the engrafo MCP server exposing exactly 7 tools.
type Server struct {
	handlers *Handlers
}

// New creates a new Server backed by the given Store.
func New(s *graph.Store) *Server {
	return &Server{handlers: NewHandlers(s)}
}

// ToolCount returns the number of registered MCP tools (always 7).
func (s *Server) ToolCount() int {
	return 0 // BLOQUEANTE: stub — returns 0 until full implementation
}

// Serve starts the MCP server on stdio and blocks until the process exits.
func (s *Server) Serve() error {
	return nil // BLOQUEANTE: stub
}
