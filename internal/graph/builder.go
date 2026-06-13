package graph

// Builder builds and updates the graph from parsed source files.
// Full implementation: feature/graph-builder.
type Builder struct {
	store *Store
}

// NewBuilder creates a Builder backed by the given Store.
func NewBuilder(s *Store) *Builder {
	return &Builder{store: s}
}

// Init indexes the entire repository rooted at repoPath.
func (b *Builder) Init(_ string) error {
	return nil
}

// Update re-parses only files changed since the last indexed commit.
func (b *Builder) Update(_ string) error {
	return nil
}
