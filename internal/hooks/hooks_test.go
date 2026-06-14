package hooks_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/hooks"
	"github.com/Jomruizgo/Engrafo/internal/parser"
)

func openTestStore(t *testing.T) *graph.Store {
	t.Helper()
	s, err := graph.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedRoot(t *testing.T, s *graph.Store) (int64, int64) {
	t.Helper()
	rootID, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "test", RelPath: ".", AbsRoot: t.TempDir(), VCS: "none",
	})
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}
	revID, err := s.CreateRevision(rootID, "git", "commit-abc")
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	return rootID, revID
}

func seedGraph(t *testing.T, s *graph.Store) {
	t.Helper()
	rootID, revID := seedRoot(t, s)
	b := graph.NewBuilder(s)
	b.UpsertFile(rootID, revID, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "user", Kind: "package", FilePath: "user.go", Language: "go"},
			{Symbol: "UserService", Kind: "class", FilePath: "user.go", Language: "go"},
		},
	})
	b.UpsertFile(rootID, revID, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "server", Kind: "package", FilePath: "server.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "server.go", ToSymbol: "user", Kind: "imports"},
		},
	})
}

func TestSessionStartMessageContainsEngrafoPrefix(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	msg := hooks.SessionStartMessage(q)

	if msg == "" {
		t.Error("want non-empty session start message")
	}
	if !strings.Contains(msg, "[engrafo]") {
		t.Errorf("want '[engrafo]' prefix in message, got %q", msg)
	}
}

func TestPreReadMessageWithDependents(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	msg := hooks.PreReadMessage(q, "user.go")

	if msg == "" {
		t.Error("want non-empty pre-read message for file with dependents")
	}
	if !strings.Contains(msg, "[engrafo]") {
		t.Errorf("want '[engrafo]' prefix in message, got %q", msg)
	}
}

func TestPreReadMessageNoDependents(t *testing.T) {
	s := openTestStore(t)
	rootID, revID := seedRoot(t, s)
	b := graph.NewBuilder(s)
	b.UpsertFile(rootID, revID, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "isolated", Kind: "package", FilePath: "isolated.go", Language: "go"},
		},
	})
	q := graph.NewQuerier(s)

	msg := hooks.PreReadMessage(q, "isolated.go")

	if msg != "" {
		t.Errorf("want empty message for file with no dependents, got %q", msg)
	}
}

func TestPreWriteMessageAlwaysReturnsMessage(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	msg := hooks.PreWriteMessage(q, "user.go", 3)

	if msg == "" {
		t.Error("want non-empty pre-write message")
	}
	if !strings.Contains(msg, "[engrafo]") {
		t.Errorf("want '[engrafo]' prefix in message, got %q", msg)
	}
}

func TestHookOutputJSON(t *testing.T) {
	expected := "test message"

	out := hooks.BuildOutput(expected)

	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("BuildOutput is not valid JSON: %v\ngot: %s", err, out)
	}
	if m["systemMessage"] != expected {
		t.Errorf("want systemMessage=%q, got %v", expected, m["systemMessage"])
	}
}

func TestHookOutputEmptyMessage(t *testing.T) {
	out := hooks.BuildOutput("")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("BuildOutput('') is not valid JSON: %s", out)
	}
	if _, ok := m["systemMessage"]; ok {
		t.Error("empty message should not produce systemMessage key in output")
	}
}
