package graph_test

import (
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
)

// seedGraph creates a minimal two-file graph:
//   user.go   — package "user", struct "UserService"
//   server.go — package "server", imports "user"
func seedGraph(t *testing.T, s *graph.Store) *graph.Builder {
	t.Helper()
	b := graph.NewBuilder(s)

	b.UpsertFile("commit-abc", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "user", Kind: "package", FilePath: "user.go", Language: "go"},
			{Symbol: "UserService", Kind: "class", FilePath: "user.go", Language: "go"},
		},
	})
	b.UpsertFile("commit-abc", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "server", Kind: "package", FilePath: "server.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "server.go", ToSymbol: "user", Kind: "imports"},
			{FromSymbol: "server.go", ToSymbol: "fmt", Kind: "imports"},
		},
	})
	return b
}

func TestQuerierDependencies(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	// Act
	deps, err := q.Dependencies("server.go")

	// Assert
	if err != nil {
		t.Fatalf("Dependencies: %v", err)
	}
	if len(deps) != 2 {
		t.Errorf("want 2 dependencies for server.go, got %d: %v", len(deps), deps)
	}
	found := map[string]bool{}
	for _, d := range deps {
		found[d.Symbol] = true
	}
	if !found["user"] {
		t.Errorf("want 'user' in dependencies, got %v", deps)
	}
	if !found["fmt"] {
		t.Errorf("want 'fmt' in dependencies, got %v", deps)
	}
}

func TestQuerierDependents(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	// Act — who depends on the "user" package?
	deps, err := q.Dependents("user.go")

	// Assert
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}
	if len(deps) == 0 {
		t.Fatal("want >=1 dependent of user.go, got 0")
	}
	found := false
	for _, d := range deps {
		if d.FilePath == "server.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("want server.go as dependent of user.go, got %v", deps)
	}
}

func TestQuerierImpact(t *testing.T) {
	// Arrange: user.go ← server.go ← handler.go (transitive)
	s := openTestStore(t)
	b := graph.NewBuilder(s)

	b.UpsertFile("commit-abc", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "user", Kind: "package", FilePath: "user.go", Language: "go"},
		},
	})
	b.UpsertFile("commit-abc", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "server", Kind: "package", FilePath: "server.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "server.go", ToSymbol: "user", Kind: "imports"},
		},
	})
	b.UpsertFile("commit-abc", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "handler", Kind: "package", FilePath: "handler.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "handler.go", ToSymbol: "server", Kind: "imports"},
		},
	})
	q := graph.NewQuerier(s)

	// Act
	affected, err := q.Impact("user.go", 3)

	// Assert
	if err != nil {
		t.Fatalf("Impact: %v", err)
	}
	// server.go at depth 1, handler.go at depth 2
	if len(affected) < 2 {
		t.Errorf("want >=2 affected files, got %d: %v", len(affected), affected)
	}
	files := map[string]bool{}
	for _, a := range affected {
		files[a.FilePath] = true
	}
	if !files["server.go"] {
		t.Errorf("want server.go in impact set, got %v", files)
	}
	if !files["handler.go"] {
		t.Errorf("want handler.go in impact set, got %v", files)
	}
}

func TestQuerierSearch(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	b := graph.NewBuilder(s)
	b.UpsertFile("commit-abc", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "UserService", Kind: "class", FilePath: "user.go", Language: "go"},
			{Symbol: "ProductService", Kind: "class", FilePath: "product.go", Language: "go"},
		},
	})
	q := graph.NewQuerier(s)

	// Act
	results, err := q.Search("UserService", 10)

	// Assert
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("want >=1 result for 'UserService', got 0")
	}
	if results[0].Symbol != "UserService" {
		t.Errorf("want UserService as top result, got %q", results[0].Symbol)
	}
}

func TestQuerierContext(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	// Act
	ctx, err := q.Context()

	// Assert
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if ctx == nil {
		t.Fatal("Context: got nil")
	}
	if ctx.TotalNodes < 2 {
		t.Errorf("want >=2 total nodes, got %d", ctx.TotalNodes)
	}
	found := false
	for _, l := range ctx.Languages {
		if l == "go" {
			found = true
		}
	}
	if !found {
		t.Errorf("want 'go' in languages, got %v", ctx.Languages)
	}
}
