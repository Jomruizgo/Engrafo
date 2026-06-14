package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	engrafo "github.com/Jomruizgo/Engrafo/internal/mcp"
	"github.com/Jomruizgo/Engrafo/internal/parser"
)

// --- helpers ---

func openTestStore(t *testing.T) *graph.Store {
	t.Helper()
	s, err := graph.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedTestRoot(t *testing.T, s *graph.Store) (int64, int64) {
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
	rootID, revID := seedTestRoot(t, s)
	b := graph.NewBuilder(s)
	b.UpsertFile(rootID, revID, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "UserService", Kind: "class", FilePath: "user.go", Language: "go"},
			{Symbol: "user", Kind: "package", FilePath: "user.go", Language: "go"},
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

// callHandler invokes a Handlers method with the given arguments and returns
// the parsed JSON payload from the first text content item.
func callHandler(
	t *testing.T,
	fn func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error),
	args map[string]any,
) map[string]any {
	t.Helper()
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = args

	result, err := fn(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}
	if len(result.Content) == 0 {
		t.Fatal("handler returned empty content")
	}
	tc, ok := mcplib.AsTextContent(result.Content[0])
	if !ok {
		t.Fatal("first content item is not TextContent")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("result text is not valid JSON: %v\ntext: %s", err, tc.Text)
	}
	return out
}

// --- tests ---

func TestCGContextHandler(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	h := engrafo.NewHandlers(s)

	out := callHandler(t, h.CGContext, nil)

	if _, ok := out["languages"]; !ok {
		t.Errorf("cg_context response missing 'languages' key; got %v", out)
	}
	if _, ok := out["stats"]; !ok {
		t.Errorf("cg_context response missing 'stats' key; got %v", out)
	}
}

func TestCGNodeHandler(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	h := engrafo.NewHandlers(s)

	out := callHandler(t, h.CGNode, map[string]any{"symbol": "UserService"})

	if _, ok := out["node"]; !ok {
		t.Errorf("cg_node response missing 'node' key; got %v", out)
	}
}

func TestCGDependentsHandler(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	h := engrafo.NewHandlers(s)

	out := callHandler(t, h.CGDependents, map[string]any{"file_path": "user.go"})

	dependents, ok := out["dependents"].([]any)
	if !ok {
		t.Fatalf("cg_dependents response missing 'dependents' array; got %v", out)
	}
	if len(dependents) == 0 {
		t.Error("want >=1 dependent of user.go, got 0")
	}
}

func TestCGDependenciesHandler(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	h := engrafo.NewHandlers(s)

	out := callHandler(t, h.CGDependencies, map[string]any{"file_path": "server.go"})

	deps, ok := out["dependencies"].([]any)
	if !ok {
		t.Fatalf("cg_dependencies response missing 'dependencies' array; got %v", out)
	}
	if len(deps) == 0 {
		t.Error("want >=1 dependency of server.go, got 0")
	}
}

func TestCGImpactHandler(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	h := engrafo.NewHandlers(s)

	out := callHandler(t, h.CGImpact, map[string]any{"file_path": "user.go"})

	if _, ok := out["affected"]; !ok {
		t.Errorf("cg_impact response missing 'affected' key; got %v", out)
	}
	if _, ok := out["total_count"]; !ok {
		t.Errorf("cg_impact response missing 'total_count' key; got %v", out)
	}
}

func TestCGSearchHandler(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	h := engrafo.NewHandlers(s)

	out := callHandler(t, h.CGSearch, map[string]any{"query": "UserService"})

	results, ok := out["results"].([]any)
	if !ok {
		t.Fatalf("cg_search response missing 'results' array; got %v", out)
	}
	if len(results) == 0 {
		t.Error("want >=1 search result for UserService, got 0")
	}
}

func TestCGAnchorHandler(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	h := engrafo.NewHandlers(s)

	out := callHandler(t, h.CGAnchor, map[string]any{
		"engram_obs_id": "obs-uuid-001",
		"symbols":       []any{"UserService"},
	})

	anchored, ok := out["anchored"].(float64)
	if !ok {
		t.Fatalf("cg_anchor response missing 'anchored' count; got %v", out)
	}
	if anchored < 1 {
		t.Errorf("want anchored >= 1, got %v", anchored)
	}
}

func TestCGDeadcodeHandler(t *testing.T) {
	s := openTestStore(t)
	rootID, revID := seedTestRoot(t, s)
	b := graph.NewBuilder(s)
	b.UpsertFile(rootID, revID, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "orphanFn", Kind: "function", FilePath: "orphan.go", Language: "go"},
		},
	})
	h := engrafo.NewHandlers(s)

	out := callHandler(t, h.CGDeadcode, nil)

	if _, ok := out["orphans"]; !ok {
		t.Errorf("cg_deadcode response missing 'orphans' key; got %v", out)
	}
	if _, ok := out["abandoned"]; !ok {
		t.Errorf("cg_deadcode response missing 'abandoned' key; got %v", out)
	}
}

func TestServerHasNineTools(t *testing.T) {
	s := openTestStore(t)

	srv := engrafo.New(s)
	count := srv.ToolCount()

	if count != 9 {
		t.Errorf("want exactly 9 MCP tools, got %d", count)
	}
}

func TestCGHistoryHandler(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	h := engrafo.NewHandlers(s)

	out := callHandler(t, h.CGHistory, map[string]any{"symbol": "UserService", "kind": "class"})

	if _, ok := out["timeline"]; !ok {
		t.Error("CGHistory missing 'timeline' key")
	}
	if _, ok := out["symbol"]; !ok {
		t.Error("CGHistory missing 'symbol' key")
	}
}
