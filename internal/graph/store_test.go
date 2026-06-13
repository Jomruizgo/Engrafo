package graph_test

import (
	"database/sql"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
)

// TestStoreSchemaCreation verifies that Open creates all required tables.
func TestStoreSchemaCreation(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")

	// Act
	store, err := graph.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	// Assert: all schema tables present
	want := []string{
		"edges", "engram_anchors", "index_meta",
		"nodes", "nodes_fts", "schema_version",
	}
	got := queryTableNames(t, store.DB())
	if !equalStringSlices(want, got) {
		t.Errorf("tables:\n  want %v\n   got %v", want, got)
	}
}

// TestStoreSchemaVersion verifies schema_version records the current version.
func TestStoreSchemaVersion(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	store, err := graph.Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	// Act
	var version int
	err = store.DB().QueryRow(
		"SELECT version FROM schema_version ORDER BY version DESC LIMIT 1",
	).Scan(&version)

	// Assert
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != graph.SchemaVersion {
		t.Errorf("schema_version: want %d, got %d", graph.SchemaVersion, version)
	}
}

// TestStoreMigration verifies that a DB at version 0 migrates to SchemaVersion without error.
func TestStoreMigration(t *testing.T) {
	// Arrange: seed a DB that has schema_version table but version=0
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	seedVersion0(t, dbPath)

	// Act: Open must migrate automatically
	store, err := graph.Open(dbPath)
	if err != nil {
		t.Fatalf("Open (migration): %v", err)
	}
	defer store.Close()

	// Assert: now at current version
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

// TestStoreBiTemporalEdge verifies the bi-temporal model:
// - insert edge with valid_until_commit = NULL (active)
// - update valid_until_commit to a commit hash (invalidate — never delete)
// - default query no longer returns it; full query does.
func TestStoreBiTemporalEdge(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	store, err := graph.Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	db := store.DB()

	fromID := insertTestNode(t, db, "FuncA", "function")
	toID := insertTestNode(t, db, "FuncB", "function")

	// Act: insert active edge
	_, err = db.Exec(`
		INSERT INTO edges (from_id, to_id, kind, valid_from_commit)
		VALUES (?, ?, 'calls', 'commit-abc')`,
		fromID, toID,
	)
	if err != nil {
		t.Fatalf("insert edge: %v", err)
	}

	// Assert: edge is active
	var count int
	db.QueryRow("SELECT COUNT(*) FROM edges WHERE valid_until_commit IS NULL").Scan(&count)
	if count != 1 {
		t.Fatalf("active edges before invalidation: want 1, got %d", count)
	}

	// Act: invalidate (bi-temporal — UPDATE not DELETE)
	_, err = db.Exec(`
		UPDATE edges
		SET valid_until_commit = 'commit-def', invalidated_reason = 'refactor'
		WHERE from_id = ? AND to_id = ? AND kind = 'calls' AND valid_until_commit IS NULL`,
		fromID, toID,
	)
	if err != nil {
		t.Fatalf("invalidate edge: %v", err)
	}

	// Assert: active query returns 0
	db.QueryRow("SELECT COUNT(*) FROM edges WHERE valid_until_commit IS NULL").Scan(&count)
	if count != 0 {
		t.Errorf("active edges after invalidation: want 0, got %d", count)
	}

	// Assert: historical query returns invalidated edge with correct commit
	var untilCommit string
	err = db.QueryRow(
		"SELECT valid_until_commit FROM edges WHERE from_id = ?", fromID,
	).Scan(&untilCommit)
	if err != nil {
		t.Fatalf("query invalidated edge: %v", err)
	}
	if untilCommit != "commit-def" {
		t.Errorf("valid_until_commit: want 'commit-def', got %q", untilCommit)
	}
}

// helpers

func queryTableNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	want := []string{"edges", "engram_anchors", "index_meta", "nodes", "nodes_fts", "schema_version"}
	placeholders := "('edges','engram_anchors','index_meta','nodes','nodes_fts','schema_version')"
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
	_ = want
	sort.Strings(names)
	return names
}

func insertTestNode(t *testing.T, db *sql.DB, symbol, kind string) int64 {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO nodes (symbol, kind, file_path, language)
		VALUES (?, ?, 'test.go', 'go')`, symbol, kind)
	if err != nil {
		t.Fatalf("insertTestNode %s: %v", symbol, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedVersion0 creates a DB with schema_version table at version 0
// and no other tables — simulates an "old" engrafo DB.
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
	db.Exec("INSERT INTO schema_version (version) VALUES (0)")
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
