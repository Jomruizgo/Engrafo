// Package ui serves the engrafo graph browser over HTTP (localhost only).
package ui

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
)

//go:embed static
var staticFiles embed.FS

// Server holds the HTTP handlers backed by a graph Store.
type Server struct {
	store   *graph.Store
	querier *graph.Querier
}

// NewServer creates a Server backed by the given Store.
func NewServer(s *graph.Store) *Server {
	return &Server{store: s, querier: graph.NewQuerier(s)}
}

// Handler returns the http.Handler for the UI (routes + static files).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	sub, _ := fs.Sub(staticFiles, "static")
	staticHandler := http.FileServer(http.FS(sub))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		staticHandler.ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/context", s.handleContext)
	mux.HandleFunc("/api/nodes", s.handleNodes)
	mux.HandleFunc("/api/node", s.handleNode)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/deadcode", s.handleDeadcode)
	mux.HandleFunc("/api/graph", s.handleGraph)

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.querier.Context()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type stats struct {
		TotalNodes int `json:"total_nodes"`
	}
	writeJSON(w, map[string]any{
		"languages": ctx.Languages,
		"top_nodes": ctx.TopNodes,
		"stats":     stats{TotalNodes: ctx.TotalNodes},
		"counts":    ctx.NodeCounts,
		"roots":     ctx.Roots,
	})
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.querier.AllNodes(0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if nodes == nil {
		nodes = []graph.NodeSummary{}
	}
	writeJSON(w, nodes)
}

func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	kind := r.URL.Query().Get("kind")
	if symbol == "" {
		http.Error(w, "missing symbol param", http.StatusBadRequest)
		return
	}
	result, err := s.querier.NodeInfo(symbol, kind, true, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	nd := result.Node
	writeJSON(w, map[string]any{
		"node": map[string]any{
			"id":        nd.ID,
			"symbol":    nd.Symbol,
			"kind":      nd.Kind,
			"file_path": nd.FilePath,
			"line_start": nd.LineStart,
			"line_end":  nd.LineEnd,
			"language":  nd.Language,
		},
		"depends_on":           result.DependsOn,
		"used_by":              result.UsedBy,
		"historical_edges":     result.HistoricalEdges,
		"anchored_observations": result.AnchoredObsIDs,
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}
	if q == "" {
		writeJSON(w, map[string]any{"results": []any{}})
		return
	}
	results, err := s.querier.Search(q, limit, "")
	if err != nil {
		writeJSON(w, map[string]any{"results": []any{}})
		return
	}
	if results == nil {
		results = []graph.SearchResult{}
	}
	writeJSON(w, map[string]any{"results": results})
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	rootName := r.URL.Query().Get("root")
	data, err := s.querier.GraphData(rootName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleDeadcode(w http.ResponseWriter, r *http.Request) {
	thresholdStr := r.URL.Query().Get("threshold_days")
	threshold := 0
	if v, err := strconv.Atoi(thresholdStr); err == nil {
		threshold = v
	}
	result, err := s.querier.Deadcode(threshold)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	orphans := result.Orphans
	abandoned := result.Abandoned
	if orphans == nil {
		orphans = []graph.OrphanNode{}
	}
	if abandoned == nil {
		abandoned = []graph.AbandonedNode{}
	}
	writeJSON(w, map[string]any{
		"orphans":   orphans,
		"abandoned": abandoned,
	})
}
