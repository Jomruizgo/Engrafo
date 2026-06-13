package graph

// QueryResult is a single node returned by a query.
type QueryResult struct {
	Symbol   string
	Kind     string
	FilePath string
	EdgeKind string
	Depth    int
}

// Dependents returns all nodes that depend on the given file or symbol.
func (s *Store) Dependents(_, _ string) ([]QueryResult, error) {
	return nil, nil
}

// Dependencies returns everything the given file or symbol depends on.
func (s *Store) Dependencies(_, _ string) ([]QueryResult, error) {
	return nil, nil
}

// Impact returns the transitive blast radius of modifying a file.
func (s *Store) Impact(_ string, _ int) ([]QueryResult, error) {
	return nil, nil
}

// Search performs FTS5 search over symbol names and signatures.
func (s *Store) Search(_ string, _ int) ([]QueryResult, error) {
	return nil, nil
}
