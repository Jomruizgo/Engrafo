package graph

import "github.com/Jomruizgo/Engrafo/internal/parser"

// Builder builds and updates the graph from parsed source files.
type Builder struct {
	store *Store
}

// NewBuilder creates a Builder backed by the given Store.
func NewBuilder(s *Store) *Builder {
	return &Builder{store: s}
}

// UpsertFile persists a single file's parse result into the graph.
// commitHash identifies the git commit at which this version of the file exists.
// Bi-temporal: edges removed from the new result are invalidated (not deleted).
func (b *Builder) UpsertFile(_ string, _ *parser.Result) error {
	return nil // BLOQUEANTE: stub — implementation in green commit
}

// Init indexes the entire repository rooted at repoPath.
func (b *Builder) Init(_ string) error {
	return nil
}

// Update re-parses only files changed since the last indexed commit.
func (b *Builder) Update(_ string) error {
	return nil
}

// SetMeta stores a key/value pair in index_meta.
func (b *Builder) SetMeta(key, value string) error {
	_, err := b.store.db.Exec(
		`INSERT INTO index_meta(key, value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	return err
}
