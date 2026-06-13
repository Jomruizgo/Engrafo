package graph_test

import (
	"path/filepath"
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
)

// seedHistoryGraph creates a node whose edges change across commits.
//
// commit-A: caller → [dep1, dep2]
// commit-B: caller → [dep1]        (dep2 disappeared)
// commit-C: caller → [dep1, dep3]  (dep3 appeared)
func seedHistoryGraph(t *testing.T) *graph.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := graph.Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	b := graph.NewBuilder(s)

	// commit-A
	b.UpsertFile("commit-A", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "caller", Kind: "function", FilePath: "a.go", Language: "go"},
			{Symbol: "dep1", Kind: "function", FilePath: "b.go", Language: "go"},
			{Symbol: "dep2", Kind: "function", FilePath: "b.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "caller", ToSymbol: "dep1", Kind: "calls"},
			{FromSymbol: "caller", ToSymbol: "dep2", Kind: "calls"},
		},
	})

	// commit-B: dep2 dropped
	b.UpsertFile("commit-B", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "caller", Kind: "function", FilePath: "a.go", Language: "go"},
			{Symbol: "dep1", Kind: "function", FilePath: "b.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "caller", ToSymbol: "dep1", Kind: "calls"},
		},
	})

	// commit-C: dep3 added
	b.UpsertFile("commit-C", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "caller", Kind: "function", FilePath: "a.go", Language: "go"},
			{Symbol: "dep1", Kind: "function", FilePath: "b.go", Language: "go"},
			{Symbol: "dep3", Kind: "function", FilePath: "c.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "caller", ToSymbol: "dep1", Kind: "calls"},
			{FromSymbol: "caller", ToSymbol: "dep3", Kind: "calls"},
		},
	})

	return s
}

func TestHistoryReturnsNodeIdentity(t *testing.T) {
	// Arrange
	s := seedHistoryGraph(t)
	q := graph.NewQuerier(s)

	// Act
	result, err := q.History("caller", "function")
	if err != nil {
		t.Fatalf("History: %v", err)
	}

	// Assert
	if result.Symbol != "caller" {
		t.Errorf("want symbol=caller, got %q", result.Symbol)
	}
	if result.Kind != "function" {
		t.Errorf("want kind=function, got %q", result.Kind)
	}
}

func TestHistoryTimelineContainsAppearAndDisappear(t *testing.T) {
	// Arrange
	s := seedHistoryGraph(t)
	q := graph.NewQuerier(s)

	// Act
	result, err := q.History("caller", "function")
	if err != nil {
		t.Fatalf("History: %v", err)
	}

	// Assert: timeline must contain at least one "appeared" and one "disappeared"
	appeared, disappeared := 0, 0
	for _, ev := range result.Timeline {
		switch ev.EventType {
		case "appeared":
			appeared++
		case "disappeared":
			disappeared++
		}
	}
	if appeared == 0 {
		t.Error("want at least one 'appeared' event in timeline")
	}
	if disappeared == 0 {
		t.Error("want at least one 'disappeared' event in timeline, dep2 was dropped at commit-B")
	}
}

func TestHistoryDep2Disappeared(t *testing.T) {
	// Arrange
	s := seedHistoryGraph(t)
	q := graph.NewQuerier(s)

	// Act
	result, err := q.History("caller", "function")
	if err != nil {
		t.Fatalf("History: %v", err)
	}

	// Assert: dep2 must appear as a "disappeared" event at commit-B
	found := false
	for _, ev := range result.Timeline {
		if ev.TargetSymbol == "dep2" && ev.EventType == "disappeared" {
			found = true
			if ev.Commit != "commit-B" {
				t.Errorf("dep2 disappeared at wrong commit: want commit-B, got %q", ev.Commit)
			}
		}
	}
	if !found {
		t.Error("dep2 must appear as 'disappeared' event in timeline")
	}
}

func TestHistoryDep3Appeared(t *testing.T) {
	// Arrange
	s := seedHistoryGraph(t)
	q := graph.NewQuerier(s)

	// Act
	result, err := q.History("caller", "function")
	if err != nil {
		t.Fatalf("History: %v", err)
	}

	// Assert: dep3 appeared at commit-C
	found := false
	for _, ev := range result.Timeline {
		if ev.TargetSymbol == "dep3" && ev.EventType == "appeared" {
			found = true
			if ev.Commit != "commit-C" {
				t.Errorf("dep3 appeared at wrong commit: want commit-C, got %q", ev.Commit)
			}
		}
	}
	if !found {
		t.Error("dep3 must appear as 'appeared' event in timeline at commit-C")
	}
}

func TestHistoryUnknownSymbolReturnsError(t *testing.T) {
	// Arrange
	s := seedHistoryGraph(t)
	q := graph.NewQuerier(s)

	// Act / Assert
	_, err := q.History("nonexistent", "")
	if err == nil {
		t.Error("want error for unknown symbol, got nil")
	}
}
