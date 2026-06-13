// Package graph manages the SQLite-backed dependency graph.
// Full implementation: feature/schema-sqlite.
package graph

// Store provides access to the engrafo SQLite database.
// Schema: schema/schema.sql — bi-temporal edges (never deleted, only invalidated).
type Store struct{}

// Open opens (or creates) the graph.db at the given path and runs pending migrations.
func Open(_ string) (*Store, error) {
	return &Store{}, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	return nil
}
