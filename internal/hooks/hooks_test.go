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

func seedGraph(t *testing.T, s *graph.Store) {
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
		},
	})
}

func TestSessionStartMessageContainsEngrafoPrefix(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	// Act
	msg := hooks.SessionStartMessage(q)

	// Assert
	if msg == "" {
		t.Error("want non-empty session start message")
	}
	if !strings.Contains(msg, "[engrafo]") {
		t.Errorf("want '[engrafo]' prefix in message, got %q", msg)
	}
}

func TestPreReadMessageWithDependents(t *testing.T) {
	// Arrange: user.go has server.go as dependent
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	// Act
	msg := hooks.PreReadMessage(q, "user.go")

	// Assert
	if msg == "" {
		t.Error("want non-empty pre-read message for file with dependents")
	}
	if !strings.Contains(msg, "[engrafo]") {
		t.Errorf("want '[engrafo]' prefix in message, got %q", msg)
	}
}

func TestPreReadMessageNoDependents(t *testing.T) {
	// Arrange: isolated file with no dependents
	s := openTestStore(t)
	b := graph.NewBuilder(s)
	b.UpsertFile("commit-abc", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "isolated", Kind: "package", FilePath: "isolated.go", Language: "go"},
		},
	})
	q := graph.NewQuerier(s)

	// Act
	msg := hooks.PreReadMessage(q, "isolated.go")

	// Assert: no dependents → no injection
	if msg != "" {
		t.Errorf("want empty message for file with no dependents, got %q", msg)
	}
}

func TestPreWriteMessageAlwaysReturnsMessage(t *testing.T) {
	// Arrange
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	// Act
	msg := hooks.PreWriteMessage(q, "user.go", 3)

	// Assert: always returns a message (may be warning or informative)
	if msg == "" {
		t.Error("want non-empty pre-write message")
	}
	if !strings.Contains(msg, "[engrafo]") {
		t.Errorf("want '[engrafo]' prefix in message, got %q", msg)
	}
}

func TestHookOutputJSON(t *testing.T) {
	// Arrange
	expected := "test message"

	// Act
	out := hooks.BuildOutput(expected)

	// Assert: result is valid JSON with systemMessage key
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("BuildOutput is not valid JSON: %v\ngot: %s", err, out)
	}
	if m["systemMessage"] != expected {
		t.Errorf("want systemMessage=%q, got %v", expected, m["systemMessage"])
	}
}

func TestHookOutputEmptyMessage(t *testing.T) {
	// Empty message → still valid JSON, no systemMessage key
	out := hooks.BuildOutput("")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("BuildOutput('') is not valid JSON: %s", out)
	}
	if _, ok := m["systemMessage"]; ok {
		t.Error("empty message should not produce systemMessage key in output")
	}
}
