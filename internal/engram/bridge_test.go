package engram_test

import (
	"path/filepath"
	"testing"

	"github.com/Jomruizgo/Engrafo/v2/internal/engram"
	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
	"github.com/Jomruizgo/Engrafo/v2/internal/parser"
)

func openStore(t *testing.T) *graph.Store {
	t.Helper()
	s, err := graph.Open(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedGraph creates a root + revision and a few nodes to anchor against.
func seedGraph(t *testing.T, s *graph.Store) {
	t.Helper()
	rootID, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "saas-auth", RelPath: ".", AbsRoot: "/saas-auth", VCS: "git",
	})
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}
	revID, err := s.CreateRevision(rootID, "git", "commit-A")
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	b := graph.NewBuilder(s)
	if err := b.UpsertFile(rootID, revID, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "AuthService", Kind: "class", FilePath: "auth.ts", Language: "typescript"},
			{Symbol: "login", Kind: "function", FilePath: "auth.ts", Language: "typescript"},
			{Symbol: "create_service_snapshot", Kind: "function", FilePath: "snapshot.py", Language: "python"},
		},
	}); err != nil {
		t.Fatalf("UpsertFile: %v", err)
	}
}

func TestAutoAnchorMatchesCodeSymbols(t *testing.T) {
	s := openStore(t)
	seedGraph(t, s)

	b := engram.New(s)
	// Prose is lowercase; the only real symbol mentioned is "AuthService".
	text := "decidimos usar Cognito en AuthService para evitar manejar tokens a mano"
	n, err := b.AutoAnchor("obs-abc123", text, "")
	if err != nil {
		t.Fatalf("AutoAnchor: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 anchor (AuthService), got %d", n)
	}

	// The anchored observation must surface from NodeInfo on that symbol.
	q := graph.NewQuerier(s)
	info, err := q.NodeInfo("AuthService", "class", false, "")
	if err != nil {
		t.Fatalf("NodeInfo: %v", err)
	}
	if len(info.AnchoredObsIDs) != 1 || info.AnchoredObsIDs[0] != "obs-abc123" {
		t.Errorf("want anchored obs 'obs-abc123' on AuthService, got %v", info.AnchoredObsIDs)
	}
}

func TestAutoAnchorMatchesPythonSnakeCase(t *testing.T) {
	s := openStore(t)
	seedGraph(t, s)

	b := engram.New(s)
	// Python codebases use snake_case; prose words won't contain underscores.
	text := "el bug estaba en create_service_snapshot cuando el tenant no existia"
	n, err := b.AutoAnchor("obs-py1", text, "")
	if err != nil {
		t.Fatalf("AutoAnchor: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 anchor (create_service_snapshot), got %d", n)
	}

	q := graph.NewQuerier(s)
	info, err := q.NodeInfo("create_service_snapshot", "function", false, "")
	if err != nil {
		t.Fatalf("NodeInfo: %v", err)
	}
	if len(info.AnchoredObsIDs) != 1 || info.AnchoredObsIDs[0] != "obs-py1" {
		t.Errorf("want obs-py1 anchored, got %v", info.AnchoredObsIDs)
	}
}

func TestAutoAnchorIgnoresProseOnlyText(t *testing.T) {
	s := openStore(t)
	seedGraph(t, s)

	b := engram.New(s)
	// No code symbols, only lowercase prose — nothing should anchor.
	n, err := b.AutoAnchor("obs-xyz", "decidimos usar una base de datos relacional para todo", "")
	if err != nil {
		t.Fatalf("AutoAnchor: %v", err)
	}
	if n != 0 {
		t.Errorf("want 0 anchors for prose-only text, got %d", n)
	}
}

func TestAutoAnchorEmptyInputs(t *testing.T) {
	s := openStore(t)
	seedGraph(t, s)
	b := engram.New(s)

	if n, _ := b.AutoAnchor("", "AuthService", ""); n != 0 {
		t.Errorf("want 0 anchors for empty obsID, got %d", n)
	}
	if n, _ := b.AutoAnchor("obs-1", "", ""); n != 0 {
		t.Errorf("want 0 anchors for empty text, got %d", n)
	}
}
