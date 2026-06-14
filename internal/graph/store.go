// Package graph manages the SQLite-backed dependency graph.
// Schema: schema/schema.sql — bi-temporal edges (never deleted, only invalidated).
package graph

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Jomruizgo/Engrafo/schema"
)

// SchemaVersion is the current schema version; incremented with each migration.
const SchemaVersion = 2

// ResolvedRoot holds the fully-resolved metadata for an indexed root.
type ResolvedRoot struct {
	Name          string
	RelPath       string
	AbsRoot       string
	RemoteURL     string
	DefaultBranch string
	VCS           string // "git" | "none"
}

// RootRow is a root as stored in the database.
type RootRow struct {
	ID             int64
	Name           string
	RelPath        string
	AbsRoot        string
	RemoteURL      string
	DefaultBranch  string
	VCS            string
	LastCommitHash string
	IndexedAt      string
}

// Store provides access to the engrafo SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) graph.db at path and runs pending migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open(sqliteDriver, path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign_keys: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// OpenRawDB opens a SQLite connection without running migrations.
// Exported for test helpers only.
func OpenRawDB(path string) (*sql.DB, error) {
	return sql.Open(sqliteDriver, path)
}

// DB returns the underlying *sql.DB.
// Exported for test helpers only.
func (s *Store) DB() *sql.DB { return s.db }

// Close releases the database connection.
func (s *Store) Close() error { return s.db.Close() }

// UpsertRoot inserts or updates a root by name and returns its ID.
func (s *Store) UpsertRoot(r ResolvedRoot) (int64, error) {
	_, err := s.db.Exec(`
		INSERT INTO roots(name, rel_path, abs_root, remote_url, default_branch, vcs)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			rel_path       = excluded.rel_path,
			abs_root       = excluded.abs_root,
			remote_url     = excluded.remote_url,
			default_branch = excluded.default_branch,
			vcs            = excluded.vcs
	`, r.Name, r.RelPath, r.AbsRoot, nullStr(r.RemoteURL), nullStr(r.DefaultBranch), r.VCS)
	if err != nil {
		return 0, fmt.Errorf("upsert root %q: %w", r.Name, err)
	}
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM roots WHERE name = ?`, r.Name).Scan(&id); err != nil {
		return 0, fmt.Errorf("get root id %q: %w", r.Name, err)
	}
	return id, nil
}

// GetRoot returns the root with the given name, or nil if not found.
func (s *Store) GetRoot(name string) (*RootRow, error) {
	var r RootRow
	err := s.db.QueryRow(`
		SELECT id, name, rel_path, abs_root,
		       COALESCE(remote_url,''), COALESCE(default_branch,''),
		       vcs, COALESCE(last_commit_hash,''), COALESCE(indexed_at,'')
		FROM roots WHERE name = ?
	`, name).Scan(&r.ID, &r.Name, &r.RelPath, &r.AbsRoot,
		&r.RemoteURL, &r.DefaultBranch, &r.VCS, &r.LastCommitHash, &r.IndexedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get root %q: %w", name, err)
	}
	return &r, nil
}

// AllRoots returns all indexed roots ordered by name.
func (s *Store) AllRoots() ([]RootRow, error) {
	rows, err := s.db.Query(`
		SELECT id, name, rel_path, abs_root,
		       COALESCE(remote_url,''), COALESCE(default_branch,''),
		       vcs, COALESCE(last_commit_hash,''), COALESCE(indexed_at,'')
		FROM roots ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("all roots: %w", err)
	}
	defer rows.Close()
	var out []RootRow
	for rows.Next() {
		var r RootRow
		if err := rows.Scan(&r.ID, &r.Name, &r.RelPath, &r.AbsRoot,
			&r.RemoteURL, &r.DefaultBranch, &r.VCS, &r.LastCommitHash, &r.IndexedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetRootIndexed updates last_commit_hash and indexed_at=now for a root.
func (s *Store) SetRootIndexed(rootID int64, lastCommit string) error {
	_, err := s.db.Exec(`
		UPDATE roots SET last_commit_hash = ?, indexed_at = datetime('now') WHERE id = ?
	`, nullStr(lastCommit), rootID)
	return err
}

// CreateRevision inserts a new revision record and returns its ID.
func (s *Store) CreateRevision(rootID int64, source, commitHash string) (int64, error) {
	res, err := s.db.Exec(`
		INSERT INTO revisions(root_id, source, commit_hash) VALUES(?, ?, ?)
	`, rootID, source, nullStr(commitHash))
	if err != nil {
		return 0, fmt.Errorf("create revision: %w", err)
	}
	return res.LastInsertId()
}

// FileChecksum returns the checksum stored in the file node for the given root+path.
// Returns "" if the node doesn't exist or has no checksum.
func (s *Store) FileChecksum(rootID int64, relPath string) (string, error) {
	var checksum string
	err := s.db.QueryRow(`
		SELECT COALESCE(checksum,'') FROM nodes
		WHERE root_id = ? AND file_path = ? AND kind = 'file'
		LIMIT 1
	`, rootID, relPath).Scan(&checksum)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return checksum, err
}

// AnchorObservations links an engram observation ID to all nodes matching the given symbols.
// If rootName != "", only nodes in that root are considered.
// Returns the number of anchors actually created.
func (s *Store) AnchorObservations(obsID string, symbols []string, rootName string) (int, error) {
	if len(symbols) == 0 {
		return 0, nil
	}

	var rootID int64
	var filterByRoot bool
	if rootName != "" {
		if err := s.db.QueryRow(`SELECT id FROM roots WHERE name = ?`, rootName).Scan(&rootID); err != nil {
			return 0, fmt.Errorf("root %q not found: %w", rootName, err)
		}
		filterByRoot = true
	}

	count := 0
	for _, sym := range symbols {
		var rows *sql.Rows
		var err error
		if filterByRoot {
			rows, err = s.db.Query(`SELECT id FROM nodes WHERE symbol = ? AND root_id = ?`, sym, rootID)
		} else {
			rows, err = s.db.Query(`SELECT id FROM nodes WHERE symbol = ?`, sym)
		}
		if err != nil {
			return count, fmt.Errorf("lookup symbol %q: %w", sym, err)
		}
		var nodeIDs []int64
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			nodeIDs = append(nodeIDs, id)
		}
		rows.Close()
		for _, nid := range nodeIDs {
			_, err := s.db.Exec(
				`INSERT OR IGNORE INTO engram_anchors(node_id, engram_obs_id) VALUES(?,?)`,
				nid, obsID,
			)
			if err != nil {
				return count, fmt.Errorf("anchor node %d: %w", nid, err)
			}
			count++
		}
	}
	return count, nil
}

// migrate ensures the schema is at SchemaVersion.
func (s *Store) migrate() error {
	var current int
	err := s.db.QueryRow(
		"SELECT version FROM schema_version ORDER BY version DESC LIMIT 1",
	).Scan(&current)

	switch {
	case err != nil:
		// No schema_version table → fresh DB: create full v2 schema.
		if _, err := s.db.Exec(schema.SQL); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
		_, err = s.db.Exec("INSERT INTO schema_version(version) VALUES(?)", SchemaVersion)
		return err

	case current == SchemaVersion:
		return nil

	case current == 1:
		// v1 → v2: structural migration (commit hashes → revision IDs).
		return s.migrateV1ToV2()

	case current < SchemaVersion:
		// Any other old version: create missing tables (IF NOT EXISTS) then bump.
		if _, err := s.db.Exec(schema.SQL); err != nil {
			return fmt.Errorf("migration v%d→v%d: %w", current, SchemaVersion, err)
		}
		_, err = s.db.Exec("INSERT OR REPLACE INTO schema_version(version) VALUES(?)", SchemaVersion)
		return err

	default:
		return nil // already at or beyond current version
	}
}

// migrateV1ToV2 performs the structural v1→v2 migration.
// Steps per §5 of plan-v2.0-multi-repo.md.
func (s *Store) migrateV1ToV2() error {
	// PRAGMA foreign_keys cannot be changed inside a transaction.
	if _, err := s.db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable fk: %w", err)
	}
	defer s.db.Exec("PRAGMA foreign_keys = ON")

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()

	// Step 1: Create roots table.
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS roots (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		name             TEXT NOT NULL UNIQUE,
		rel_path         TEXT NOT NULL,
		abs_root         TEXT NOT NULL,
		remote_url       TEXT,
		default_branch   TEXT,
		vcs              TEXT NOT NULL DEFAULT 'git',
		last_commit_hash TEXT,
		indexed_at       DATETIME
	)`); err != nil {
		return fmt.Errorf("create roots: %w", err)
	}

	// Step 2: Derive root #1 from index_meta.
	var repoRoot, lastCommitHash, indexedAt string
	tx.QueryRow(`SELECT COALESCE(value,'') FROM index_meta WHERE key='repo_root'`).Scan(&repoRoot)
	tx.QueryRow(`SELECT COALESCE(value,'') FROM index_meta WHERE key='last_commit_hash'`).Scan(&lastCommitHash)
	tx.QueryRow(`SELECT COALESCE(value,'') FROM index_meta WHERE key='indexed_at'`).Scan(&indexedAt)

	rootName := filepath.Base(repoRoot)
	if rootName == "" || rootName == "." || rootName == "/" || rootName == "\\" {
		rootName = "root"
	}

	vcs := "none"
	if repoRoot != "" {
		if _, statErr := os.Stat(filepath.Join(repoRoot, ".git")); statErr == nil {
			vcs = "git"
		}
	}

	// For the roots table: last_commit_hash should be NULL when value is "init" or "".
	var lcHashNullable, indexedAtNullable interface{}
	if lastCommitHash != "" && lastCommitHash != "init" {
		lcHashNullable = lastCommitHash
	}
	if indexedAt != "" {
		indexedAtNullable = indexedAt
	}

	res, err := tx.Exec(`INSERT INTO roots(name, rel_path, abs_root, vcs, last_commit_hash, indexed_at)
		VALUES(?, '.', ?, ?, ?, ?)`, rootName, repoRoot, vcs, lcHashNullable, indexedAtNullable)
	if err != nil {
		return fmt.Errorf("insert root: %w", err)
	}
	rootID, _ := res.LastInsertId()

	// Step 3: Create revisions table and populate from edge commit hashes.
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS revisions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		root_id     INTEGER NOT NULL REFERENCES roots(id) ON DELETE CASCADE,
		source      TEXT NOT NULL,
		commit_hash TEXT,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create revisions: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_revisions_root ON revisions(root_id)`); err != nil {
		return fmt.Errorf("create revisions index: %w", err)
	}

	// Collect distinct hashes ordered by first appearance.
	hashRows, err := tx.Query(`
		SELECT hash, MIN(created_at) AS first_seen FROM (
			SELECT valid_from_commit AS hash, created_at FROM edges
			WHERE valid_from_commit IS NOT NULL AND valid_from_commit != ''
			UNION ALL
			SELECT valid_until_commit AS hash, created_at FROM edges
			WHERE valid_until_commit IS NOT NULL AND valid_until_commit != ''
		) GROUP BY hash ORDER BY first_seen
	`)
	if err != nil {
		return fmt.Errorf("collect edge hashes: %w", err)
	}
	type hashEntry struct{ hash, firstSeen string }
	var hashes []hashEntry
	for hashRows.Next() {
		var h, fs string
		hashRows.Scan(&h, &fs)
		hashes = append(hashes, hashEntry{h, fs})
	}
	hashRows.Close()

	hashToRevID := make(map[string]int64)

	for _, he := range hashes {
		src := "git"
		var chNullable interface{}
		if he.hash == "init" {
			src = "init"
			// commit_hash stays NULL
		} else {
			chNullable = he.hash
		}
		res2, err := tx.Exec(
			`INSERT INTO revisions(root_id, source, commit_hash) VALUES(?, ?, ?)`,
			rootID, src, chNullable,
		)
		if err != nil {
			return fmt.Errorf("insert revision for %q: %w", he.hash, err)
		}
		revID, _ := res2.LastInsertId()
		hashToRevID[he.hash] = revID
		if he.hash == "init" {
			hashToRevID[""] = revID
		}
	}

	// If no edges exist, create a single init revision so the DB is usable.
	if len(hashes) == 0 {
		res2, err := tx.Exec(`INSERT INTO revisions(root_id, source) VALUES(?, 'init')`, rootID)
		if err != nil {
			return fmt.Errorf("insert init revision: %w", err)
		}
		initRevID, _ := res2.LastInsertId()
		hashToRevID["init"] = initRevID
		hashToRevID[""] = initRevID
	} else if _, ok := hashToRevID[""]; !ok {
		// Ensure empty-string hash maps to something (treat as init).
		hashToRevID[""] = hashToRevID["init"]
	}

	// Step 4: Add root_id to nodes and update all rows to root #1.
	if _, err := tx.Exec(`ALTER TABLE nodes ADD COLUMN root_id INTEGER REFERENCES roots(id)`); err != nil {
		return fmt.Errorf("alter nodes add root_id: %w", err)
	}
	if _, err := tx.Exec(`UPDATE nodes SET root_id = ?`, rootID); err != nil {
		return fmt.Errorf("update nodes root_id: %w", err)
	}
	tx.Exec(`DROP INDEX IF EXISTS idx_nodes_symbol_file`)
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_root_symbol_file
		ON nodes(root_id, symbol, file_path, kind)`); err != nil {
		return fmt.Errorf("create nodes index: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_root ON nodes(root_id)`); err != nil {
		return fmt.Errorf("create nodes root index: %w", err)
	}

	// Step 5: Rebuild edges table (TEXT commit cols → INTEGER rev FK cols).
	if _, err := tx.Exec(`CREATE TABLE edges_new (
		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		from_id            INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		to_id              INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		kind               TEXT NOT NULL,
		valid_from_rev     INTEGER NOT NULL REFERENCES revisions(id),
		valid_until_rev    INTEGER REFERENCES revisions(id),
		invalidated_reason TEXT,
		created_at         DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create edges_new: %w", err)
	}

	edgeRows, err := tx.Query(`
		SELECT id, from_id, to_id, kind,
		       valid_from_commit,
		       valid_until_commit,
		       COALESCE(invalidated_reason,''),
		       created_at
		FROM edges
	`)
	if err != nil {
		return fmt.Errorf("read old edges: %w", err)
	}
	type oldEdge struct {
		id, fromID, toID       int64
		kind, validFrom        string
		validUntil             sql.NullString
		invalidatedReason      string
		createdAt              string
	}
	var oldEdges []oldEdge
	for edgeRows.Next() {
		var e oldEdge
		if err := edgeRows.Scan(&e.id, &e.fromID, &e.toID, &e.kind,
			&e.validFrom, &e.validUntil, &e.invalidatedReason, &e.createdAt); err != nil {
			edgeRows.Close()
			return fmt.Errorf("scan edge: %w", err)
		}
		oldEdges = append(oldEdges, e)
	}
	edgeRows.Close()
	if err := edgeRows.Err(); err != nil {
		return fmt.Errorf("iterate edges: %w", err)
	}

	for _, e := range oldEdges {
		fromRevID, ok := hashToRevID[e.validFrom]
		if !ok {
			// Unknown hash: create ad-hoc revision.
			src := "git"
			var chNull interface{} = e.validFrom
			if e.validFrom == "init" || e.validFrom == "" {
				src = "init"
				chNull = nil
			}
			res2, err := tx.Exec(
				`INSERT INTO revisions(root_id, source, commit_hash) VALUES(?, ?, ?)`,
				rootID, src, chNull,
			)
			if err != nil {
				return fmt.Errorf("ad-hoc revision for %q: %w", e.validFrom, err)
			}
			fromRevID, _ = res2.LastInsertId()
			hashToRevID[e.validFrom] = fromRevID
		}

		var untilRevNullable interface{}
		if e.validUntil.Valid && e.validUntil.String != "" {
			untilRevID, ok := hashToRevID[e.validUntil.String]
			if !ok {
				src := "git"
				var chNull interface{} = e.validUntil.String
				if e.validUntil.String == "init" {
					src = "init"
					chNull = nil
				}
				res2, err := tx.Exec(
					`INSERT INTO revisions(root_id, source, commit_hash) VALUES(?, ?, ?)`,
					rootID, src, chNull,
				)
				if err != nil {
					return fmt.Errorf("ad-hoc revision for until %q: %w", e.validUntil.String, err)
				}
				untilRevID, _ = res2.LastInsertId()
				hashToRevID[e.validUntil.String] = untilRevID
			}
			untilRevNullable = untilRevID
		}

		var invReasonNullable interface{}
		if e.invalidatedReason != "" {
			invReasonNullable = e.invalidatedReason
		}

		if _, err := tx.Exec(`
			INSERT INTO edges_new(id, from_id, to_id, kind, valid_from_rev, valid_until_rev, invalidated_reason, created_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		`, e.id, e.fromID, e.toID, e.kind, fromRevID, untilRevNullable, invReasonNullable, e.createdAt); err != nil {
			return fmt.Errorf("insert edge_new %d: %w", e.id, err)
		}
	}

	if _, err := tx.Exec(`DROP TABLE edges`); err != nil {
		return fmt.Errorf("drop old edges: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE edges_new RENAME TO edges`); err != nil {
		return fmt.Errorf("rename edges_new: %w", err)
	}
	tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_edges_unique_active
		ON edges(from_id, to_id, kind) WHERE valid_until_rev IS NULL`)
	tx.Exec(`CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_id)`)
	tx.Exec(`CREATE INDEX IF NOT EXISTS idx_edges_to   ON edges(to_id)`)

	// Step 6: Update schema_version.
	if _, err := tx.Exec(`INSERT OR REPLACE INTO schema_version(version) VALUES(2)`); err != nil {
		return fmt.Errorf("update schema_version: %w", err)
	}

	return tx.Commit()
}

// nullStr returns nil when s is empty, allowing SQL NULL storage.
func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
