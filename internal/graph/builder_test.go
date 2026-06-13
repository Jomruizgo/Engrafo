package graph_test

import (
	"path/filepath"
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
)

// openTestStore creates a fresh Store in a temp dir for testing.
// It is shared across all test files in this package.
func openTestStore(t *testing.T) *graph.Store {
	t.Helper()
	s, err := graph.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestBuilderUpsertNodes(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	b := graph.NewBuilder(s)
	result := &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "MyFunc", Kind: "function", FilePath: "main.go", Language: "go", LineStart: 1, LineEnd: 5},
			{Symbol: "MyService", Kind: "class", FilePath: "main.go", Language: "go", LineStart: 7, LineEnd: 20},
		},
	}

	// Act
	err := b.UpsertFile("commit-abc", result)

	// Assert
	if err != nil {
		t.Fatalf("UpsertFile: %v", err)
	}
	// 2 explicit symbols + 1 auto-created file node
	var count int
	s.DB().QueryRow(`SELECT count(*) FROM nodes WHERE file_path = 'main.go'`).Scan(&count)
	if count != 3 {
		t.Errorf("want 3 nodes (2 symbols + 1 file node), got %d", count)
	}
}

func TestBuilderUpsertIsIdempotent(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	b := graph.NewBuilder(s)
	result := &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "MyFunc", Kind: "function", FilePath: "main.go", Language: "go"},
		},
	}

	// Act: upsert same result twice
	b.UpsertFile("commit-abc", result)
	err := b.UpsertFile("commit-abc", result)

	// Assert
	if err != nil {
		t.Fatalf("second UpsertFile: %v", err)
	}
	var count int
	s.DB().QueryRow(`SELECT count(*) FROM nodes WHERE symbol = 'MyFunc'`).Scan(&count)
	if count != 1 {
		t.Errorf("idempotent: want 1 node, got %d", count)
	}
}

func TestBuilderCreatesExternalNodeForUnresolvedEdge(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	b := graph.NewBuilder(s)
	result := &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "main", Kind: "package", FilePath: "main.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "main.go", ToSymbol: "fmt", Kind: "imports"},
		},
	}

	// Act
	err := b.UpsertFile("commit-abc", result)

	// Assert
	if err != nil {
		t.Fatalf("UpsertFile: %v", err)
	}
	var edgeCount int
	s.DB().QueryRow(`SELECT count(*) FROM edges WHERE valid_until_commit IS NULL`).Scan(&edgeCount)
	if edgeCount != 1 {
		t.Errorf("want 1 active edge, got %d", edgeCount)
	}
	var extCount int
	s.DB().QueryRow(`SELECT count(*) FROM nodes WHERE symbol = 'fmt' AND kind = 'external'`).Scan(&extCount)
	if extCount != 1 {
		t.Errorf("want 1 external node for 'fmt', got %d", extCount)
	}
}

func TestBuilderResolvesEdgeToKnownNode(t *testing.T) {
	// Arrange: user.go has "user" package; server.go imports "user"
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

	// Act
	var toFilePath string
	err := s.DB().QueryRow(`
		SELECT n.file_path FROM edges e
		JOIN nodes n ON n.id = e.to_id
		WHERE e.valid_until_commit IS NULL AND n.symbol = 'user'
	`).Scan(&toFilePath)

	// Assert
	if err != nil {
		t.Fatalf("query edge target: %v", err)
	}
	if toFilePath != "user.go" {
		t.Errorf("edge should resolve to user.go, got %q", toFilePath)
	}
}

func TestBuilderInvalidatesRemovedEdge(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	b := graph.NewBuilder(s)
	v1 := &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "main", Kind: "package", FilePath: "main.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "main.go", ToSymbol: "fmt", Kind: "imports"},
			{FromSymbol: "main.go", ToSymbol: "errors", Kind: "imports"},
		},
	}
	v2 := &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "main", Kind: "package", FilePath: "main.go", Language: "go"},
		},
		Edges: []parser.Edge{
			// errors import removed
			{FromSymbol: "main.go", ToSymbol: "fmt", Kind: "imports"},
		},
	}

	// Act
	b.UpsertFile("commit-v1", v1)
	err := b.UpsertFile("commit-v2", v2)

	// Assert
	if err != nil {
		t.Fatalf("UpsertFile v2: %v", err)
	}
	var activeCount int
	s.DB().QueryRow(`SELECT count(*) FROM edges WHERE valid_until_commit IS NULL`).Scan(&activeCount)
	if activeCount != 1 {
		t.Errorf("want 1 active edge after removal, got %d", activeCount)
	}
	var invalidCount int
	s.DB().QueryRow(`SELECT count(*) FROM edges WHERE valid_until_commit = 'commit-v2'`).Scan(&invalidCount)
	if invalidCount != 1 {
		t.Errorf("want 1 edge invalidated with commit-v2, got %d", invalidCount)
	}
}
