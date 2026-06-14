package ui_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
	"github.com/Jomruizgo/Engrafo/internal/ui"
)

func openSeededStore(t *testing.T) *graph.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := graph.Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	rootID, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "test", RelPath: ".", AbsRoot: dir, VCS: "none",
	})
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}
	revID, err := s.CreateRevision(rootID, "git", "abc123")
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	b := graph.NewBuilder(s)
	err = b.UpsertFile(rootID, revID, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "processOrder", Kind: "function", FilePath: "order.go", Language: "go"},
			{Symbol: "Order", Kind: "class", FilePath: "order.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "processOrder", ToSymbol: "Order", Kind: "uses"},
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	return s
}

func doGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestUIHomeReturnsHTML(t *testing.T) {
	// Arrange
	s := openSeededStore(t)
	h := ui.NewServer(s).Handler()

	// Act
	w := doGet(t, h, "/")

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct == "" || ct[:9] != "text/html" {
		t.Errorf("want text/html Content-Type, got %q", ct)
	}
	if body := w.Body.String(); len(body) < 200 {
		t.Errorf("HTML body too short (%d bytes), probably empty", len(body))
	}
}

func TestUIContextEndpoint(t *testing.T) {
	// Arrange
	s := openSeededStore(t)
	h := ui.NewServer(s).Handler()

	// Act
	w := doGet(t, h, "/api/context")

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("not valid JSON: %s", w.Body.String())
	}
	if _, ok := m["languages"]; !ok {
		t.Error("missing 'languages' key in /api/context response")
	}
	if _, ok := m["stats"]; !ok {
		t.Error("missing 'stats' key in /api/context response")
	}
}

func TestUINodesEndpoint(t *testing.T) {
	// Arrange
	s := openSeededStore(t)
	h := ui.NewServer(s).Handler()

	// Act
	w := doGet(t, h, "/api/nodes")

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	var nodes []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &nodes); err != nil {
		t.Fatalf("not valid JSON array: %s", w.Body.String())
	}
	if len(nodes) < 1 {
		t.Error("want at least one node in /api/nodes response")
	}
}

func TestUINodeDetailEndpoint(t *testing.T) {
	// Arrange
	s := openSeededStore(t)
	h := ui.NewServer(s).Handler()

	// Act
	w := doGet(t, h, "/api/node?symbol=processOrder&kind=function")

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("not valid JSON: %s", w.Body.String())
	}
	if _, ok := m["node"]; !ok {
		t.Error("missing 'node' key in /api/node response")
	}
}

func TestUISearchEndpoint(t *testing.T) {
	// Arrange
	s := openSeededStore(t)
	h := ui.NewServer(s).Handler()

	// Act
	w := doGet(t, h, "/api/search?q=processOrder&limit=10")

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("not valid JSON: %s", w.Body.String())
	}
	if _, ok := m["results"]; !ok {
		t.Error("missing 'results' key in /api/search response")
	}
}

func TestUIDeadcodeEndpoint(t *testing.T) {
	// Arrange
	s := openSeededStore(t)
	h := ui.NewServer(s).Handler()

	// Act
	w := doGet(t, h, "/api/deadcode")

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("not valid JSON: %s", w.Body.String())
	}
	if _, ok := m["orphans"]; !ok {
		t.Error("missing 'orphans' key in /api/deadcode response")
	}
	if _, ok := m["abandoned"]; !ok {
		t.Error("missing 'abandoned' key in /api/deadcode response")
	}
}

func TestUINotFoundReturns404(t *testing.T) {
	// Arrange
	s := openSeededStore(t)
	h := ui.NewServer(s).Handler()

	// Act
	w := doGet(t, h, "/nonexistent-path")

	// Assert
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown path, got %d", w.Code)
	}
}
