package graph_test

import (
	"path/filepath"
	"testing"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
	"github.com/Jomruizgo/Engrafo/v2/internal/parser"
)

// seedHistoryGraph creates a node whose edges change across commits.
//
// commit-A: caller â†’ [dep1, dep2]
// commit-B: caller â†’ [dep1]        (dep2 disappeared)
// commit-C: caller â†’ [dep1, dep3]  (dep3 appeared)
func seedHistoryGraph(t *testing.T) *graph.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := graph.Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	rootID, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "test", RelPath: ".", AbsRoot: dir, VCS: "git",
	})
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}

	revA, err := s.CreateRevision(rootID, "git", "commit-A")
	if err != nil {
		t.Fatalf("CreateRevision commit-A: %v", err)
	}
	revB, err := s.CreateRevision(rootID, "git", "commit-B")
	if err != nil {
		t.Fatalf("CreateRevision commit-B: %v", err)
	}
	revC, err := s.CreateRevision(rootID, "git", "commit-C")
	if err != nil {
		t.Fatalf("CreateRevision commit-C: %v", err)
	}

	b := graph.NewBuilder(s)

	// commit-A
	b.UpsertFile(rootID, revA, "", &parser.Result{
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
	b.UpsertFile(rootID, revB, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "caller", Kind: "function", FilePath: "a.go", Language: "go"},
			{Symbol: "dep1", Kind: "function", FilePath: "b.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "caller", ToSymbol: "dep1", Kind: "calls"},
		},
	})

	// commit-C: dep3 added
	b.UpsertFile(rootID, revC, "", &parser.Result{
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
	s := seedHistoryGraph(t)
	q := graph.NewQuerier(s)

	result, err := q.History("caller", "function", "")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if result.Symbol != "caller" {
		t.Errorf("want symbol=caller, got %q", result.Symbol)
	}
	if result.Kind != "function" {
		t.Errorf("want kind=function, got %q", result.Kind)
	}
}

func TestHistoryTimelineContainsAppearAndDisappear(t *testing.T) {
	s := seedHistoryGraph(t)
	q := graph.NewQuerier(s)

	result, err := q.History("caller", "function", "")
	if err != nil {
		t.Fatalf("History: %v", err)
	}

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
	s := seedHistoryGraph(t)
	q := graph.NewQuerier(s)

	result, err := q.History("caller", "function", "")
	if err != nil {
		t.Fatalf("History: %v", err)
	}

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
	s := seedHistoryGraph(t)
	q := graph.NewQuerier(s)

	result, err := q.History("caller", "function", "")
	if err != nil {
		t.Fatalf("History: %v", err)
	}

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
	s := seedHistoryGraph(t)
	q := graph.NewQuerier(s)

	_, err := q.History("nonexistent", "", "")
	if err == nil {
		t.Error("want error for unknown symbol, got nil")
	}
}
