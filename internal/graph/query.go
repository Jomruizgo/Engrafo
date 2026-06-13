package graph

// DependentNode is a node returned by dependency queries.
type DependentNode struct {
	Symbol   string
	FilePath string
	Kind     string
	EdgeKind string
	Depth    int
}

// SearchResult is a node returned by FTS5 symbol search.
type SearchResult struct {
	Symbol   string
	Kind     string
	FilePath string
}

// NodeSummary is a compact node representation for project context.
type NodeSummary struct {
	Symbol   string
	Kind     string
	FilePath string
	InDegree int
}

// ProjectContext is the high-level summary returned by cg_context.
type ProjectContext struct {
	TotalNodes int
	Languages  []string
	TopNodes   []NodeSummary
	NodeCounts map[string]int
}

// QueryResult is a single node returned by a query (kept for backwards compat).
type QueryResult = DependentNode

// Querier runs read-only queries against the graph Store.
type Querier struct {
	store *Store
}

// NewQuerier creates a Querier backed by the given Store.
func NewQuerier(s *Store) *Querier {
	return &Querier{store: s}
}

// Dependencies returns all active outgoing edges from nodes in filePath.
func (q *Querier) Dependencies(_ string) ([]DependentNode, error) {
	return nil, nil // BLOQUEANTE: stub
}

// Dependents returns all nodes that have active edges pointing into filePath.
func (q *Querier) Dependents(_ string) ([]DependentNode, error) {
	return nil, nil // BLOQUEANTE: stub
}

// Impact returns the transitive blast radius of modifying filePath up to depth hops.
func (q *Querier) Impact(_ string, _ int) ([]DependentNode, error) {
	return nil, nil // BLOQUEANTE: stub
}

// Search performs FTS5 search over symbol names and returns up to limit results.
func (q *Querier) Search(_ string, _ int) ([]SearchResult, error) {
	return nil, nil // BLOQUEANTE: stub
}

// Context returns aggregate project statistics.
func (q *Querier) Context() (*ProjectContext, error) {
	return nil, nil // BLOQUEANTE: stub
}
