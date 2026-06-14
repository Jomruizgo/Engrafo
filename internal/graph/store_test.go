package graph_test

import (
	"database/sql"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
)

// TestStoreSchemaCreation verifies that Open creates all required tables.
func TestStoreSchemaCreation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")

	store, err := graph.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	want := []string{
		"edges", "engram_anchors", "index_meta",
		"nodes", "nodes_fts", "revisions", "roots", "schema_version",
	}
	got := queryTableNames(t, store.DB())
	if !equalStringSlices(want, got) {
		t.Errorf("tables:\n  want %v\n   got %v", want, got)
	}
}

// TestStoreSchemaVersion verifies schema_version records SchemaVersion=2.
func TestStoreSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	store, err := graph.Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	var version int
	err = store.DB().QueryRow(
		"SELECT version FROM schema_version ORDER BY version DESC LIMIT 1",
	).Scan(&version)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != graph.SchemaVersion {
		t.Errorf("schema_version: want %d, got %d", graph.SchemaVersion, version)
	}
}

// TestStoreMigration verifies that a DB at version 0 migrates to SchemaVersion without error.
func TestStoreMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	seedVersion0(t, dbPath)

	store, err := graph.Open(dbPath)
	if err != nil {
		t.Fatalf("Open (migration): %v", err)
	}
	defer store.Close()

	var version int
	if err := store.DB().QueryRow(
		"SELECT MAX(version) FROM schema_version",
	).Scan(&version); err != nil {
		t.Fatalf("query version after migration: %v", err)
	}
	if version != graph.SchemaVersion {
		t.Errorf("post-migration version: want %d, got %d", graph.SchemaVersion, version)
	}
}

// TestStoreBiTemporalEdge verifies the bi-temporal model with revision IDs.
func TestStoreBiTemporalEdge(t *testing.T) {
	dir := t.TempDir()
	s, err := graph.Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	rootID, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "test", RelPath: ".", AbsRoot: dir, VCS: "none",
	})
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}

	revFrom, err := s.CreateRevision(rootID, "git", "commit-abc")
	if err != nil {
		t.Fatalf("CreateRevision from: %v", err)
	}
	revUntil, err := s.CreateRevision(rootID, "git", "commit-def")
	if err != nil {
		t.Fatalf("CreateRevision until: %v", err)
	}

	db := s.DB()
	fromID := insertTestNode(t, db, rootID, "FuncA", "function")
	toID := insertTestNode(t, db, rootID, "FuncB", "function")

	// Insert active edge.
	_, err = db.Exec(`
		INSERT INTO edges(from_id, to_id, kind, valid_from_rev)
		VALUES(?, ?, 'calls', ?)`, fromID, toID, revFrom)
	if err != nil {
		t.Fatalf("insert edge: %v", err)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM edges WHERE valid_until_rev IS NULL").Scan(&count)
	if count != 1 {
		t.Fatalf("active edges before invalidation: want 1, got %d", count)
	}

	// Invalidate (bi-temporal — UPDATE not DELETE).
	_, err = db.Exec(`
		UPDATE edges SET valid_until_rev=?, invalidated_reason='refactor'
		WHERE from_id=? AND to_id=? AND kind='calls' AND valid_until_rev IS NULL`,
		revUntil, fromID, toID)
	if err != nil {
		t.Fatalf("invalidate edge: %v", err)
	}

	db.QueryRow("SELECT COUNT(*) FROM edges WHERE valid_until_rev IS NULL").Scan(&count)
	if count != 0 {
		t.Errorf("active edges after invalidation: want 0, got %d", count)
	}

	var untilRev int64
	err = db.QueryRow("SELECT valid_until_rev FROM edges WHERE from_id=?", fromID).Scan(&untilRev)
	if err != nil {
		t.Fatalf("query invalidated edge: %v", err)
	}
	if untilRev != revUntil {
		t.Errorf("valid_until_rev: want %d, got %d", revUntil, untilRev)
	}
}

// TestMigrationV1ToV2 — test #1: migrar DB v1 con nodos+aristas al schema v2.
func TestMigrationV1ToV2(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "v1.db")
	seedV1DB(t, dbPath)

	s, err := graph.Open(dbPath)
	if err != nil {
		t.Fatalf("Open after migration: %v", err)
	}
	defer s.Close()
	db := s.DB()

	// Roots table has exactly 1 row.
	var rootCount int
	db.QueryRow(`SELECT COUNT(*) FROM roots`).Scan(&rootCount)
	if rootCount != 1 {
		t.Errorf("want 1 root, got %d", rootCount)
	}

	// Root name = basename of /test/repo = "repo".
	var rootName string
	db.QueryRow(`SELECT name FROM roots WHERE id=1`).Scan(&rootName)
	if rootName != "repo" {
		t.Errorf("want root name='repo', got %q", rootName)
	}

	// Revisions created for distinct commits (commit-abc, commit-def).
	var revCount int
	db.QueryRow(`SELECT COUNT(*) FROM revisions`).Scan(&revCount)
	if revCount < 2 {
		t.Errorf("want >=2 revisions (commit-abc + commit-def), got %d", revCount)
	}

	// All nodes have root_id=1.
	var nodesWithRoot, totalNodes int
	db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE root_id=1`).Scan(&nodesWithRoot)
	db.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&totalNodes)
	if nodesWithRoot != totalNodes || totalNodes == 0 {
		t.Errorf("all nodes must have root_id=1: total=%d, with root=%d", totalNodes, nodesWithRoot)
	}

	// Active edge (a.go → FuncB) is still active.
	var activeEdges int
	db.QueryRow(`SELECT COUNT(*) FROM edges WHERE valid_until_rev IS NULL`).Scan(&activeEdges)
	if activeEdges != 1 {
		t.Errorf("want 1 active edge after migration, got %d", activeEdges)
	}

	// Historical edge (a.go → FuncA, invalidated at commit-def) still historical.
	var historicalEdges int
	db.QueryRow(`SELECT COUNT(*) FROM edges WHERE valid_until_rev IS NOT NULL`).Scan(&historicalEdges)
	if historicalEdges != 1 {
		t.Errorf("want 1 historical edge after migration, got %d", historicalEdges)
	}

	// FTS still works.
	q := graph.NewQuerier(s)
	results, err := q.Search("FuncA", 5)
	if err != nil {
		t.Fatalf("FTS search after migration: %v", err)
	}
	if len(results) == 0 {
		t.Error("FTS must find FuncA after migration")
	}

	// Anchor preserved.
	var anchorCount int
	db.QueryRow(`SELECT COUNT(*) FROM engram_anchors`).Scan(&anchorCount)
	if anchorCount != 1 {
		t.Errorf("want 1 anchor preserved, got %d", anchorCount)
	}

	// schema_version = 2.
	var version int
	db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version)
	if version != 2 {
		t.Errorf("want schema_version=2, got %d", version)
	}
}

// TestBiTemporalNoGit — test #4 (Store/Builder level): verifica bi-temporalidad sin git.
func TestBiTemporalNoGit(t *testing.T) {
	dir := t.TempDir()
	s, err := graph.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Crear raíz vcs=none.
	rootID, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "myproject", RelPath: ".", AbsRoot: dir, VCS: "none",
	})
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}

	// Revisión inicial (source='init').
	initRevID, err := s.CreateRevision(rootID, "init", "")
	if err != nil {
		t.Fatalf("CreateRevision init: %v", err)
	}

	b := graph.NewBuilder(s)

	// v1: a.go llama a callee1 y callee2.
	if err := b.UpsertFile(rootID, initRevID, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "caller", Kind: "function", FilePath: "a.go", Language: "go"},
			{Symbol: "callee1", Kind: "function", FilePath: "b.go", Language: "go"},
			{Symbol: "callee2", Kind: "function", FilePath: "c.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "a.go", ToSymbol: "callee1", Kind: "calls"},
			{FromSymbol: "a.go", ToSymbol: "callee2", Kind: "calls"},
		},
	}); err != nil {
		t.Fatalf("UpsertFile init: %v", err)
	}

	// Segunda revisión (source='checksum').
	csRevID, err := s.CreateRevision(rootID, "checksum", "")
	if err != nil {
		t.Fatalf("CreateRevision checksum: %v", err)
	}

	// v2: a.go ya no llama a callee2.
	if err := b.UpsertFile(rootID, csRevID, "", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "caller", Kind: "function", FilePath: "a.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "a.go", ToSymbol: "callee1", Kind: "calls"},
		},
	}); err != nil {
		t.Fatalf("UpsertFile update: %v", err)
	}

	db := s.DB()

	// Arista a callee1 sigue activa.
	var activeEdges int
	db.QueryRow(`SELECT COUNT(*) FROM edges WHERE valid_until_rev IS NULL`).Scan(&activeEdges)
	if activeEdges != 1 {
		t.Errorf("want 1 active edge after update, got %d", activeEdges)
	}

	// Arista a callee2 invalidada con csRevID.
	var invalidRevID int64
	err = db.QueryRow(`
		SELECT e.valid_until_rev FROM edges e
		JOIN nodes n ON n.id = e.to_id
		WHERE n.symbol = 'callee2' AND e.valid_until_rev IS NOT NULL
	`).Scan(&invalidRevID)
	if err != nil {
		t.Fatalf("query invalidated edge: %v", err)
	}
	if invalidRevID != csRevID {
		t.Errorf("want invalidated with revID=%d (checksum rev), got %d", csRevID, invalidRevID)
	}

	// La revisión es source='checksum' y commit_hash=NULL.
	var revSource string
	var revCommitHash sql.NullString
	db.QueryRow(`SELECT source, commit_hash FROM revisions WHERE id=?`, csRevID).Scan(&revSource, &revCommitHash)
	if revSource != "checksum" {
		t.Errorf("want revision source='checksum', got %q", revSource)
	}
	if revCommitHash.Valid {
		t.Errorf("want revision commit_hash=NULL for checksum rev, got %q", revCommitHash.String)
	}
}

// helpers

func queryTableNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	placeholders := "('edges','engram_anchors','index_meta','nodes','nodes_fts','revisions','roots','schema_version')"
	rows, err := db.Query(
		"SELECT name FROM sqlite_master WHERE type='table' AND name IN " + placeholders + " ORDER BY name",
	)
	if err != nil {
		t.Fatalf("queryTableNames: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func insertTestNode(t *testing.T, db *sql.DB, rootID int64, symbol, kind string) int64 {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO nodes(root_id, symbol, kind, file_path, language)
		VALUES(?, ?, ?, 'test.go', 'go')`, rootID, symbol, kind)
	if err != nil {
		t.Fatalf("insertTestNode %s: %v", symbol, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedVersion0 creates a DB with schema_version table at version 0 and no other tables.
func seedVersion0(t *testing.T, path string) {
	t.Helper()
	db, err := graph.OpenRawDB(path)
	if err != nil {
		t.Fatalf("seedVersion0 OpenRawDB: %v", err)
	}
	defer db.Close()
	db.Exec(`CREATE TABLE schema_version (
		version    INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec("INSERT INTO schema_version(version) VALUES(0)")
}

// seedV1DB creates a v1 DB with nodes, edges (active + historical), FTS, and an anchor.
func seedV1DB(t *testing.T, path string) {
	t.Helper()
	db, err := graph.OpenRawDB(path)
	if err != nil {
		t.Fatalf("seedV1DB OpenRawDB: %v", err)
	}
	defer db.Close()

	db.Exec(`CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	db.Exec(`INSERT INTO schema_version VALUES(1, '2024-01-01')`)

	db.Exec(`CREATE TABLE nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		symbol TEXT NOT NULL, kind TEXT NOT NULL, file_path TEXT NOT NULL,
		line_start INTEGER, line_end INTEGER, signature TEXT,
		language TEXT NOT NULL, checksum TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE UNIQUE INDEX idx_nodes_symbol_file ON nodes(symbol, file_path, kind)`)
	db.Exec(`CREATE INDEX idx_nodes_file ON nodes(file_path)`)
	db.Exec(`CREATE INDEX idx_nodes_symbol ON nodes(symbol)`)

	db.Exec(`CREATE TABLE edges (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		to_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		kind TEXT NOT NULL,
		valid_from_commit TEXT NOT NULL,
		valid_until_commit TEXT,
		invalidated_reason TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE UNIQUE INDEX idx_edges_unique_active ON edges(from_id, to_id, kind) WHERE valid_until_commit IS NULL`)
	db.Exec(`CREATE INDEX idx_edges_from ON edges(from_id)`)
	db.Exec(`CREATE INDEX idx_edges_to   ON edges(to_id)`)

	db.Exec(`CREATE VIRTUAL TABLE nodes_fts USING fts5(symbol, file_path, signature, content=nodes, content_rowid=id)`)
	db.Exec(`CREATE TRIGGER nodes_fts_ai AFTER INSERT ON nodes BEGIN
		INSERT INTO nodes_fts(rowid, symbol, file_path, signature) VALUES (new.id, new.symbol, new.file_path, new.signature);
	END`)
	db.Exec(`CREATE TRIGGER nodes_fts_ad AFTER DELETE ON nodes BEGIN
		INSERT INTO nodes_fts(nodes_fts, rowid, symbol, file_path, signature) VALUES ('delete', old.id, old.symbol, old.file_path, old.signature);
	END`)
	db.Exec(`CREATE TRIGGER nodes_fts_au AFTER UPDATE ON nodes BEGIN
		INSERT INTO nodes_fts(nodes_fts, rowid, symbol, file_path, signature) VALUES ('delete', old.id, old.symbol, old.file_path, old.signature);
		INSERT INTO nodes_fts(rowid, symbol, file_path, signature) VALUES (new.id, new.symbol, new.file_path, new.signature);
	END`)

	db.Exec(`CREATE TABLE engram_anchors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		engram_obs_id TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE INDEX idx_anchors_node ON engram_anchors(node_id)`)
	db.Exec(`CREATE INDEX idx_anchors_obs  ON engram_anchors(engram_obs_id)`)

	db.Exec(`CREATE TABLE index_meta (key TEXT PRIMARY KEY, value TEXT)`)
	db.Exec(`INSERT INTO index_meta VALUES('repo_root', '/test/repo')`)
	db.Exec(`INSERT INTO index_meta VALUES('last_commit_hash', 'commit-abc')`)
	db.Exec(`INSERT INTO index_meta VALUES('indexed_at', '2024-01-01 00:00:00')`)

	// Insert nodes.
	db.Exec(`INSERT INTO nodes(symbol, kind, file_path, language) VALUES('FuncA', 'function', 'a.go', 'go')`)
	db.Exec(`INSERT INTO nodes(symbol, kind, file_path, language) VALUES('FuncB', 'function', 'b.go', 'go')`)
	db.Exec(`INSERT INTO nodes(symbol, kind, file_path, language) VALUES('a.go', 'file', 'a.go', 'go')`)

	var aFileID, bNodeID, aNodeID int64
	db.QueryRow(`SELECT id FROM nodes WHERE symbol='a.go'`).Scan(&aFileID)
	db.QueryRow(`SELECT id FROM nodes WHERE symbol='FuncB'`).Scan(&bNodeID)
	db.QueryRow(`SELECT id FROM nodes WHERE symbol='FuncA'`).Scan(&aNodeID)

	// Active edge: a.go → FuncB (commit-abc).
	db.Exec(`INSERT INTO edges(from_id, to_id, kind, valid_from_commit) VALUES(?, ?, 'calls', 'commit-abc')`, aFileID, bNodeID)
	// Historical edge: a.go → FuncA (commit-abc..commit-def).
	db.Exec(`INSERT INTO edges(from_id, to_id, kind, valid_from_commit, valid_until_commit) VALUES(?, ?, 'calls', 'commit-abc', 'commit-def')`, aFileID, aNodeID)

	// Anchor on FuncA.
	db.Exec(`INSERT INTO engram_anchors(node_id, engram_obs_id) VALUES(?, 'obs-001')`, aNodeID)
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
