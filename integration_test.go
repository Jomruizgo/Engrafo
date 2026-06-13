package engrafo_test

// Integration tests for v1.0 success criteria (PRD section "Criterios de éxito").
// These tests cross package boundaries to verify end-to-end data flow.

import (
	"path/filepath"
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
	"github.com/Jomruizgo/Engrafo/internal/parser/extractors"
)

func openIntegrationStore(t *testing.T) *graph.Store {
	t.Helper()
	s, err := graph.Open(filepath.Join(t.TempDir(), "integration.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestBitemporalEdgeInvalidation — PRD criterion 6:
// after removing a dependency in a later commit, Dependents() returns nothing,
// but NodeInfo(include_invalidated=true) exposes the historical edge.
func TestBitemporalEdgeInvalidation(t *testing.T) {
	// Arrange
	s := openIntegrationStore(t)
	b := graph.NewBuilder(s)

	// Commit A: server.go imports user
	if err := b.UpsertFile("commit-A", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "user", Kind: "package", FilePath: "user.go", Language: "go"},
		},
	}); err != nil {
		t.Fatalf("upsert user.go commit-A: %v", err)
	}
	if err := b.UpsertFile("commit-A", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "server", Kind: "package", FilePath: "server.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "server.go", ToSymbol: "user", Kind: "imports"},
		},
	}); err != nil {
		t.Fatalf("upsert server.go commit-A: %v", err)
	}

	// Commit B: server.go drops the import
	if err := b.UpsertFile("commit-B", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "server", Kind: "package", FilePath: "server.go", Language: "go"},
		},
		Edges: []parser.Edge{},
	}); err != nil {
		t.Fatalf("upsert server.go commit-B: %v", err)
	}

	q := graph.NewQuerier(s)

	// Act: active query — edge must be invisible
	deps, err := q.Dependents("user.go")
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}

	// Assert: no active dependents
	if len(deps) != 0 {
		t.Errorf("want 0 active dependents after invalidation, got %d", len(deps))
	}

	// Act: historical query — the edge originates from the file node "server.go",
	// so query NodeInfo on the file node (FromSymbol in edges is always the file path).
	result, err := q.NodeInfo("server.go", "file", true)
	if err != nil {
		t.Fatalf("NodeInfo(include_invalidated=true): %v", err)
	}

	// Assert: historical edge present
	if len(result.HistoricalEdges) == 0 {
		t.Fatal("want historical edge to user after invalidation, got none")
	}
	if result.HistoricalEdges[0].Symbol != "user" {
		t.Errorf("want historical edge to 'user', got %q", result.HistoricalEdges[0].Symbol)
	}
}

// TestParserFixturesProduceNodes — PRD criterion 2:
// verifies that the three supported languages produce expected node types from fixture files.
func TestParserFixturesProduceNodes(t *testing.T) {
	p := parser.New(
		&extractors.GoExtractor{},
		&extractors.TypeScriptExtractor{},
		&extractors.PythonExtractor{},
	)

	cases := []struct {
		fixture  string
		wantKind string
	}{
		{"testdata/fixtures/go/simple.go", "function"},
		{"testdata/fixtures/typescript/simple.ts", "function"},
		{"testdata/fixtures/python/simple.py", "function"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(filepath.Base(tc.fixture), func(t *testing.T) {
			result, err := p.ParseFile(tc.fixture)
			if err != nil {
				t.Fatalf("ParseFile(%s): %v", tc.fixture, err)
			}
			if len(result.Nodes) == 0 {
				t.Fatalf("want nodes from %s, got none", tc.fixture)
			}
			found := false
			for _, n := range result.Nodes {
				if n.Kind == tc.wantKind {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("want node of kind %q from %s, got: %+v", tc.wantKind, tc.fixture, result.Nodes)
			}
		})
	}
}

// TestEndToEndInitAndQuery — PRD criterion 2:
// full pipeline: parse real fixture → build graph → NodeInfo resolves correctly.
func TestEndToEndInitAndQuery(t *testing.T) {
	// Arrange
	s := openIntegrationStore(t)
	b := graph.NewBuilder(s)
	p := parser.New(&extractors.GoExtractor{})

	result, err := p.ParseFile("testdata/fixtures/go/simple.go")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	for i := range result.Nodes {
		result.Nodes[i].FilePath = "simple.go"
	}
	if err := b.UpsertFile("commit-A", result); err != nil {
		t.Fatalf("UpsertFile: %v", err)
	}

	// Act
	q := graph.NewQuerier(s)
	info, err := q.NodeInfo("UserService", "class", false)
	if err != nil {
		t.Fatalf("NodeInfo(UserService): %v", err)
	}

	// Assert
	if info.Node.Symbol != "UserService" {
		t.Errorf("want symbol 'UserService', got %q", info.Node.Symbol)
	}
	if info.Node.Kind != "class" {
		t.Errorf("want kind 'class', got %q", info.Node.Kind)
	}
	if info.Node.FilePath != "simple.go" {
		t.Errorf("want file_path 'simple.go', got %q", info.Node.FilePath)
	}
}

// TestContextReflectsIndexedData — PRD criterion 2:
// verifies cg_context returns correct counts after indexing.
func TestContextReflectsIndexedData(t *testing.T) {
	// Arrange
	s := openIntegrationStore(t)
	b := graph.NewBuilder(s)

	b.UpsertFile("commit-A", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "user", Kind: "package", FilePath: "user.go", Language: "go"},
			{Symbol: "UserService", Kind: "class", FilePath: "user.go", Language: "go"},
			{Symbol: "GetName", Kind: "method", FilePath: "user.go", Language: "go"},
		},
	})

	// Act
	q := graph.NewQuerier(s)
	ctx, err := q.Context()
	if err != nil {
		t.Fatalf("Context: %v", err)
	}

	// Assert
	if ctx.TotalNodes < 3 {
		t.Errorf("want ≥3 nodes, got %d", ctx.TotalNodes)
	}
	if len(ctx.Languages) == 0 {
		t.Error("want at least one language in context, got none")
	}
}
