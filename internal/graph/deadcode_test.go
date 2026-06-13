package graph_test

import (
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
)

// seedDeadcodeGraph builds a graph with:
//   - "orphan"    — function that never had incoming edges
//   - "used"      — function with an active incoming edge
//   - "abandoned" — function that had incoming edges in commit-A, none in commit-B
func seedDeadcodeGraph(t *testing.T, s *graph.Store) {
	t.Helper()
	b := graph.NewBuilder(s)

	// Commit-A: caller.go references both "used" and "abandoned"
	b.UpsertFile("commit-A", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "orphan", Kind: "function", FilePath: "orphan.go", Language: "go"},
			{Symbol: "used", Kind: "function", FilePath: "used.go", Language: "go"},
			{Symbol: "abandoned", Kind: "function", FilePath: "abandoned.go", Language: "go"},
		},
	})
	b.UpsertFile("commit-A", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "caller", Kind: "function", FilePath: "caller.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "caller.go", ToSymbol: "used", Kind: "calls"},
			{FromSymbol: "caller.go", ToSymbol: "abandoned", Kind: "calls"},
		},
	})

	// Commit-B: caller.go drops the reference to "abandoned"
	b.UpsertFile("commit-B", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "caller", Kind: "function", FilePath: "caller.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "caller.go", ToSymbol: "used", Kind: "calls"},
		},
	})
}

func TestDeadcodeOrphans(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	seedDeadcodeGraph(t, s)
	q := graph.NewQuerier(s)

	// Act
	result, err := q.Deadcode(0)
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}
	if result == nil {
		t.Fatal("Deadcode: got nil result")
	}

	// Assert: "orphan" must appear in orphans list
	found := false
	for _, o := range result.Orphans {
		if o.Symbol == "orphan" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("want 'orphan' in orphans list, got %+v", result.Orphans)
	}

	// "used" must NOT appear in orphans (it has an active edge)
	for _, o := range result.Orphans {
		if o.Symbol == "used" {
			t.Errorf("'used' must not be in orphans — it has an active incoming edge")
		}
	}
}

func TestDeadcodeAbandoned(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	seedDeadcodeGraph(t, s)
	q := graph.NewQuerier(s)

	// Act
	result, err := q.Deadcode(0)
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}
	if result == nil {
		t.Fatal("Deadcode: got nil result")
	}

	// Assert: "abandoned" must appear in abandoned list with peak_incoming_edges >= 1
	found := false
	for _, a := range result.Abandoned {
		if a.Symbol == "abandoned" {
			found = true
			if a.PeakIncomingEdges < 1 {
				t.Errorf("want peak_incoming_edges >= 1, got %d", a.PeakIncomingEdges)
			}
			break
		}
	}
	if !found {
		t.Errorf("want 'abandoned' in abandoned list, got %+v", result.Abandoned)
	}

	// "used" must NOT appear in abandoned (it still has active edges)
	for _, a := range result.Abandoned {
		if a.Symbol == "used" {
			t.Errorf("'used' must not be in abandoned — it has an active incoming edge")
		}
	}
}

func TestDeadcodeActiveNodeExcluded(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	seedDeadcodeGraph(t, s)
	q := graph.NewQuerier(s)

	// Act
	result, err := q.Deadcode(0)
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}
	if result == nil {
		t.Fatal("Deadcode: got nil result")
	}

	// Assert: "used" appears in neither orphans nor abandoned
	for _, o := range result.Orphans {
		if o.Symbol == "used" {
			t.Errorf("active 'used' must not be in orphans")
		}
	}
	for _, a := range result.Abandoned {
		if a.Symbol == "used" {
			t.Errorf("active 'used' must not be in abandoned")
		}
	}
}

func TestDeadcodeExcludesFileAndPackageNodes(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	seedDeadcodeGraph(t, s)
	q := graph.NewQuerier(s)

	// Act
	result, err := q.Deadcode(0)
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}
	if result == nil {
		t.Fatal("Deadcode: got nil result")
	}

	// Assert: no file, package, or external nodes in output
	check := func(sym, kind string) {
		if kind == "file" || kind == "package" || kind == "external" {
			t.Errorf("deadcode must exclude kind=%q (symbol=%q)", kind, sym)
		}
	}
	for _, o := range result.Orphans {
		check(o.Symbol, o.Kind)
	}
	for _, a := range result.Abandoned {
		check(a.Symbol, a.Kind)
	}
}
