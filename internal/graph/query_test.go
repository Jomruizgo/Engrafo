package graph_test

import (
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
)

// seedGraph creates a minimal two-file graph:
//
//	user.go   — package "user", struct "UserService"
//	server.go — package "server", imports "user"
func seedGraph(t *testing.T, s *graph.Store) *graph.Builder {
	t.Helper()
	rootID := testSeedRoot(t, s)
	revID := testSeedRevision(t, s, rootID, "commit-abc")
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
			{FromSymbol: "server.go", ToSymbol: "fmt", Kind: "imports"},
		},
	})
	return b
}

func TestQuerierDependencies(t *testing.T) {
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	deps, err := q.Dependencies("server.go", "")
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
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	deps, err := q.Dependents("user.go", "")
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
	s := openTestStore(t)
	rootID := testSeedRoot(t, s)
	revID := testSeedRevision(t, s, rootID, "commit-abc")
	b := graph.NewBuilder(s)

	b.UpsertFile(rootID, revID, "", &parser.Result{
		Nodes: []parser.Node{
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
	b.UpsertFile(rootID, revID, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "handler", Kind: "package", FilePath: "handler.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "handler.go", ToSymbol: "server", Kind: "imports"},
		},
	})
	q := graph.NewQuerier(s)

	affected, err := q.Impact("user.go", 3, "")
	if err != nil {
		t.Fatalf("Impact: %v", err)
	}
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
	s := openTestStore(t)
	rootID := testSeedRoot(t, s)
	revID := testSeedRevision(t, s, rootID, "commit-abc")
	b := graph.NewBuilder(s)
	b.UpsertFile(rootID, revID, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "UserService", Kind: "class", FilePath: "user.go", Language: "go"},
			{Symbol: "ProductService", Kind: "class", FilePath: "product.go", Language: "go"},
		},
	})
	q := graph.NewQuerier(s)

	results, err := q.Search("UserService", 10, "")
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
	s := openTestStore(t)
	seedGraph(t, s)
	q := graph.NewQuerier(s)

	ctx, err := q.Context()
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
	// Context debe incluir Roots.
	if len(ctx.Roots) == 0 {
		t.Error("want >=1 root in Context.Roots, got 0")
	}
}

// TestQuerierRootScoping — test #7: con/sin rootName; campo Root poblado.
func TestQuerierRootScoping(t *testing.T) {
	s := openTestStore(t)

	// Crear dos raíces con el mismo file_path y distintos símbolos.
	rootA, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "svc-a", RelPath: ".", AbsRoot: "/a", VCS: "none",
	})
	if err != nil {
		t.Fatalf("UpsertRoot A: %v", err)
	}
	rootB, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "svc-b", RelPath: ".", AbsRoot: "/b", VCS: "none",
	})
	if err != nil {
		t.Fatalf("UpsertRoot B: %v", err)
	}

	revA, _ := s.CreateRevision(rootA, "init", "")
	revB, _ := s.CreateRevision(rootB, "init", "")
	b := graph.NewBuilder(s)

	b.UpsertFile(rootA, revA, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "Handler", Kind: "function", FilePath: "api.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "api.go", ToSymbol: "Handler", Kind: "calls"},
		},
	})
	b.UpsertFile(rootB, revB, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "Worker", Kind: "function", FilePath: "api.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "api.go", ToSymbol: "Worker", Kind: "calls"},
		},
	})

	q := graph.NewQuerier(s)

	t.Run("sin rootName: devuelve resultados de ambas raíces", func(t *testing.T) {
		results, err := q.Search("Handler", 10, "")
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		found := false
		for _, r := range results {
			if r.Symbol == "Handler" {
				found = true
				if r.Root != "svc-a" {
					t.Errorf("want Root=svc-a for Handler, got %q", r.Root)
				}
			}
		}
		if !found {
			t.Error("want Handler in results without rootName filter")
		}
	})

	t.Run("con rootName: solo devuelve de esa raíz", func(t *testing.T) {
		deps, err := q.Dependencies("api.go", "svc-a")
		if err != nil {
			t.Fatalf("Dependencies: %v", err)
		}
		for _, d := range deps {
			if d.Root != "svc-a" {
				t.Errorf("rootName=svc-a: got node from root %q", d.Root)
			}
		}

		// svc-b no debe aparecer en dependencias de svc-a.
		for _, d := range deps {
			if d.Symbol == "Worker" {
				t.Error("Worker (svc-b) should not appear in svc-a dependencies")
			}
		}
	})

	t.Run("NodeInfo Root field poblado", func(t *testing.T) {
		info, err := q.NodeInfo("Handler", "function", false, "svc-a")
		if err != nil {
			t.Fatalf("NodeInfo: %v", err)
		}
		if info.Node.Root != "svc-a" {
			t.Errorf("NodeDetail.Root: want svc-a, got %q", info.Node.Root)
		}
	})

	t.Run("Context Roots incluye ambas raíces", func(t *testing.T) {
		ctx, err := q.Context()
		if err != nil {
			t.Fatalf("Context: %v", err)
		}
		rootNames := make(map[string]bool)
		for _, r := range ctx.Roots {
			rootNames[r.Name] = true
		}
		if !rootNames["svc-a"] {
			t.Error("want svc-a in Context.Roots")
		}
		if !rootNames["svc-b"] {
			t.Error("want svc-b in Context.Roots")
		}
	})
}
