package graph_test

import (
	"path/filepath"
	"testing"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
	"github.com/Jomruizgo/Engrafo/v2/internal/parser"
)

// openTestStore creates a fresh Store in a temp dir for testing.
// Shared across all test files in the graph_test package.
func openTestStore(t *testing.T) *graph.Store {
	t.Helper()
	s, err := graph.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// testSeedRoot creates a test root and returns its ID.
func testSeedRoot(t *testing.T, s *graph.Store) int64 {
	t.Helper()
	id, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "test", RelPath: ".", AbsRoot: "/test-root", VCS: "none",
	})
	if err != nil {
		t.Fatalf("testSeedRoot: %v", err)
	}
	return id
}

// testSeedRevision creates a revision and returns its ID.
func testSeedRevision(t *testing.T, s *graph.Store, rootID int64, commitHash string) int64 {
	t.Helper()
	src := "git"
	if commitHash == "" || commitHash == "init" {
		src = "init"
		commitHash = ""
	}
	id, err := s.CreateRevision(rootID, src, commitHash)
	if err != nil {
		t.Fatalf("testSeedRevision(%q): %v", commitHash, err)
	}
	return id
}

func TestBuilderUpsertNodes(t *testing.T) {
	s := openTestStore(t)
	rootID := testSeedRoot(t, s)
	revID := testSeedRevision(t, s, rootID, "commit-abc")
	b := graph.NewBuilder(s)
	result := &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "MyFunc", Kind: "function", FilePath: "main.go", Language: "go", LineStart: 1, LineEnd: 5},
			{Symbol: "MyService", Kind: "class", FilePath: "main.go", Language: "go", LineStart: 7, LineEnd: 20},
		},
	}

	if err := b.UpsertFile(rootID, revID, "", result); err != nil {
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
	s := openTestStore(t)
	rootID := testSeedRoot(t, s)
	revID := testSeedRevision(t, s, rootID, "commit-abc")
	b := graph.NewBuilder(s)
	result := &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "MyFunc", Kind: "function", FilePath: "main.go", Language: "go"},
		},
	}

	b.UpsertFile(rootID, revID, "", result)
	if err := b.UpsertFile(rootID, revID, "", result); err != nil {
		t.Fatalf("second UpsertFile: %v", err)
	}
	var count int
	s.DB().QueryRow(`SELECT count(*) FROM nodes WHERE symbol = 'MyFunc'`).Scan(&count)
	if count != 1 {
		t.Errorf("idempotent: want 1 node, got %d", count)
	}
}

func TestBuilderCreatesExternalNodeForUnresolvedEdge(t *testing.T) {
	s := openTestStore(t)
	rootID := testSeedRoot(t, s)
	revID := testSeedRevision(t, s, rootID, "commit-abc")
	b := graph.NewBuilder(s)
	result := &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "main", Kind: "package", FilePath: "main.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "main.go", ToSymbol: "fmt", Kind: "imports"},
		},
	}

	if err := b.UpsertFile(rootID, revID, "", result); err != nil {
		t.Fatalf("UpsertFile: %v", err)
	}
	var edgeCount int
	s.DB().QueryRow(`SELECT count(*) FROM edges WHERE valid_until_rev IS NULL`).Scan(&edgeCount)
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

	var toFilePath string
	err := s.DB().QueryRow(`
		SELECT n.file_path FROM edges e
		JOIN nodes n ON n.id = e.to_id
		WHERE e.valid_until_rev IS NULL AND n.symbol = 'user'
	`).Scan(&toFilePath)
	if err != nil {
		t.Fatalf("query edge target: %v", err)
	}
	if toFilePath != "user.go" {
		t.Errorf("edge should resolve to user.go, got %q", toFilePath)
	}
}

// TestCrossRootIsolation â€” test #2: dos raÃ­ces con mismo file_path y symbol no colisionan.
func TestCrossRootIsolation(t *testing.T) {
	s := openTestStore(t)

	rootA, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "root-a", RelPath: ".", AbsRoot: "/a", VCS: "none",
	})
	if err != nil {
		t.Fatalf("UpsertRoot A: %v", err)
	}
	rootB, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "root-b", RelPath: ".", AbsRoot: "/b", VCS: "none",
	})
	if err != nil {
		t.Fatalf("UpsertRoot B: %v", err)
	}

	revA, err := s.CreateRevision(rootA, "init", "")
	if err != nil {
		t.Fatalf("CreateRevision A: %v", err)
	}
	revB, err := s.CreateRevision(rootB, "init", "")
	if err != nil {
		t.Fatalf("CreateRevision B: %v", err)
	}

	b := graph.NewBuilder(s)

	// Mismos file_path + symbol en ambas raÃ­ces.
	shared := &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "Process", Kind: "function", FilePath: "main.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "main.go", ToSymbol: "Process", Kind: "calls"},
		},
	}
	if err := b.UpsertFile(rootA, revA, "", shared); err != nil {
		t.Fatalf("UpsertFile A: %v", err)
	}
	if err := b.UpsertFile(rootB, revB, "", shared); err != nil {
		t.Fatalf("UpsertFile B: %v", err)
	}

	db := s.DB()

	// Deben existir 2 nodos con symbol='Process' (uno por raÃ­z).
	var processCount int
	db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE symbol='Process' AND kind='function'`).Scan(&processCount)
	if processCount != 2 {
		t.Errorf("want 2 'Process' nodes (one per root), got %d", processCount)
	}

	// Cada raÃ­z debe tener exactamente 1 nodo 'Process'.
	var inA, inB int
	db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE symbol='Process' AND root_id=?`, rootA).Scan(&inA)
	db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE symbol='Process' AND root_id=?`, rootB).Scan(&inB)
	if inA != 1 {
		t.Errorf("root-a: want 1 'Process' node, got %d", inA)
	}
	if inB != 1 {
		t.Errorf("root-b: want 1 'Process' node, got %d", inB)
	}

	// Las aristas de la raÃ­z A deben apuntar solo a nodos de la raÃ­z A.
	var crossEdges int
	db.QueryRow(`
		SELECT COUNT(*) FROM edges e
		JOIN nodes fn ON fn.id = e.from_id
		JOIN nodes tn ON tn.id = e.to_id
		WHERE fn.root_id != tn.root_id AND e.valid_until_rev IS NULL
	`).Scan(&crossEdges)
	if crossEdges != 0 {
		t.Errorf("cross-root edges must be 0, got %d", crossEdges)
	}

	// Verificar que resolveOrCreateNode no crea external stubs cross-raÃ­z:
	// el nodo 'Process' de rootA debe resolverse a id de rootA, no de rootB.
	var idA, idB int64
	db.QueryRow(`SELECT id FROM nodes WHERE symbol='Process' AND root_id=?`, rootA).Scan(&idA)
	db.QueryRow(`SELECT id FROM nodes WHERE symbol='Process' AND root_id=?`, rootB).Scan(&idB)
	if idA == 0 || idB == 0 || idA == idB {
		t.Errorf("nodes must be distinct: idA=%d, idB=%d", idA, idB)
	}

	// Aristas de rootA apuntan a nodo de rootA.
	var edgeToInA int
	db.QueryRow(`
		SELECT COUNT(*) FROM edges e
		JOIN nodes fn ON fn.id = e.from_id
		JOIN nodes tn ON tn.id = e.to_id
		WHERE fn.root_id=? AND tn.id=?
	`, rootA, idA).Scan(&edgeToInA)
	if edgeToInA != 1 {
		t.Errorf("root-a edge should point to root-a's Process node, got %d", edgeToInA)
	}
}

// TestBuilderInvalidateFile verifica que InvalidateFile invalida todas las aristas activas de un archivo.
func TestBuilderInvalidateFile(t *testing.T) {
	s := openTestStore(t)
	rootID := testSeedRoot(t, s)
	revInit := testSeedRevision(t, s, rootID, "init")
	revDel := testSeedRevision(t, s, rootID, "")
	b := graph.NewBuilder(s)

	// Indexar archivo con dos aristas activas.
	if err := b.UpsertFile(rootID, revInit, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "caller", Kind: "function", FilePath: "caller.go", Language: "go"},
			{Symbol: "dep1", Kind: "function", FilePath: "dep1.go", Language: "go"},
			{Symbol: "dep2", Kind: "function", FilePath: "dep2.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "caller.go", ToSymbol: "dep1", Kind: "calls"},
			{FromSymbol: "caller.go", ToSymbol: "dep2", Kind: "calls"},
		},
	}); err != nil {
		t.Fatalf("UpsertFile: %v", err)
	}

	var activeCount int
	s.DB().QueryRow(`SELECT COUNT(*) FROM edges WHERE valid_until_rev IS NULL`).Scan(&activeCount)
	if activeCount != 2 {
		t.Fatalf("want 2 active edges before invalidation, got %d", activeCount)
	}

	// Eliminar el archivo â†’ invalidar todas sus aristas.
	if err := b.InvalidateFile(rootID, revDel, "caller.go"); err != nil {
		t.Fatalf("InvalidateFile: %v", err)
	}

	s.DB().QueryRow(`SELECT COUNT(*) FROM edges WHERE valid_until_rev IS NULL`).Scan(&activeCount)
	if activeCount != 0 {
		t.Errorf("want 0 active edges after InvalidateFile, got %d", activeCount)
	}

	var invalidatedCount int
	s.DB().QueryRow(`SELECT COUNT(*) FROM edges WHERE valid_until_rev=?`, revDel).Scan(&invalidatedCount)
	if invalidatedCount != 2 {
		t.Errorf("want 2 edges invalidated with revDel, got %d", invalidatedCount)
	}
}

func TestBuilderInvalidatesRemovedEdge(t *testing.T) {
	s := openTestStore(t)
	rootID := testSeedRoot(t, s)
	revV1 := testSeedRevision(t, s, rootID, "commit-v1")
	revV2 := testSeedRevision(t, s, rootID, "commit-v2")
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
			{FromSymbol: "main.go", ToSymbol: "fmt", Kind: "imports"},
		},
	}

	b.UpsertFile(rootID, revV1, "", v1)
	if err := b.UpsertFile(rootID, revV2, "", v2); err != nil {
		t.Fatalf("UpsertFile v2: %v", err)
	}
	var activeCount int
	s.DB().QueryRow(`SELECT count(*) FROM edges WHERE valid_until_rev IS NULL`).Scan(&activeCount)
	if activeCount != 1 {
		t.Errorf("want 1 active edge after removal, got %d", activeCount)
	}
	var invalidCount int
	s.DB().QueryRow(`SELECT count(*) FROM edges WHERE valid_until_rev = ?`, revV2).Scan(&invalidCount)
	if invalidCount != 1 {
		t.Errorf("want 1 edge invalidated with revV2=%d, got %d", revV2, invalidCount)
	}
}
