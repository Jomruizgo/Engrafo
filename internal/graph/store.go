// Package graph manages the SQLite-backed dependency graph.
// Schema: schema/schema.sql — bi-temporal edges (never deleted, only invalidated).
package graph

import (
	"database/sql"
	"fmt"

	"github.com/Jomruizgo/Engrafo/schema"
)

// SchemaVersion is the current schema version; incremented with each migration.
const SchemaVersion = 1

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
	// Enforce FK constraints — SQLite disables them by default.
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

// migrate ensures the schema is at SchemaVersion.
// Strategy: all DDL uses IF NOT EXISTS, so re-running on a partial schema is safe.
func (s *Store) migrate() error {
	var current int
	err := s.db.QueryRow(
		"SELECT version FROM schema_version ORDER BY version DESC LIMIT 1",
	).Scan(&current)

	switch {
	case err != nil:
		// schema_version table missing → fresh database: create full schema.
		if _, err := s.db.Exec(schema.SQL); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
		_, err = s.db.Exec(
			"INSERT INTO schema_version (version) VALUES (?)", SchemaVersion,
		)
		return err

	case current < SchemaVersion:
		// Older version present: re-run schema SQL (all CREATE … IF NOT EXISTS)
		// then upsert the version row.
		if _, err := s.db.Exec(schema.SQL); err != nil {
			return fmt.Errorf("migration v%d→v%d: %w", current, SchemaVersion, err)
		}
		_, err = s.db.Exec(
			"INSERT OR REPLACE INTO schema_version (version) VALUES (?)", SchemaVersion,
		)
		return err

	default:
		return nil // already at current version
	}
}
