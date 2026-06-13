package graph

import "fmt"

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

// NodeDetail holds the full metadata for a single graph node.
type NodeDetail struct {
	ID        int64
	Symbol    string
	Kind      string
	FilePath  string
	LineStart int
	LineEnd   int
	Language  string
}

// NodeInfoResult holds a node's details plus its incoming/outgoing edges and anchors.
type NodeInfoResult struct {
	Node                NodeDetail
	DependsOn           []DependentNode
	UsedBy              []DependentNode
	HistoricalEdges     []DependentNode
	AnchoredObsIDs      []string
}

// QueryResult is an alias kept for backwards compatibility.
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
func (q *Querier) Dependencies(filePath string) ([]DependentNode, error) {
	rows, err := q.store.db.Query(`
		SELECT n2.symbol, n2.file_path, n2.kind, e.kind
		FROM edges e
		JOIN nodes n1 ON n1.id = e.from_id
		JOIN nodes n2 ON n2.id = e.to_id
		WHERE n1.file_path = ?
		  AND e.valid_until_commit IS NULL
	`, filePath)
	if err != nil {
		return nil, fmt.Errorf("dependencies query: %w", err)
	}
	defer rows.Close()
	var out []DependentNode
	for rows.Next() {
		var d DependentNode
		if err := rows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.EdgeKind); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Dependents returns all nodes that have active edges pointing into nodes of filePath.
func (q *Querier) Dependents(filePath string) ([]DependentNode, error) {
	rows, err := q.store.db.Query(`
		SELECT DISTINCT n1.symbol, n1.file_path, n1.kind, e.kind
		FROM edges e
		JOIN nodes n1 ON n1.id = e.from_id
		JOIN nodes n2 ON n2.id = e.to_id
		WHERE n2.file_path = ?
		  AND e.valid_until_commit IS NULL
	`, filePath)
	if err != nil {
		return nil, fmt.Errorf("dependents query: %w", err)
	}
	defer rows.Close()
	var out []DependentNode
	for rows.Next() {
		var d DependentNode
		if err := rows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.EdgeKind); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Impact returns the transitive blast radius of modifying filePath up to depth hops.
// Uses a recursive CTE that expands file-level: if any node in file X is impacted,
// all nodes in file X are considered impacted (so edges from any of them are followed).
func (q *Querier) Impact(filePath string, depth int) ([]DependentNode, error) {
	if depth <= 0 {
		depth = 3
	}
	rows, err := q.store.db.Query(`
		WITH RECURSIVE impact(node_id, file_path, depth) AS (
			-- seed: all nodes in the target file
			SELECT id, file_path, 0
			FROM nodes
			WHERE file_path = ?
			UNION
			-- traverse: for each impacted node, expand to all peers in the same file,
			-- then find nodes that have edges pointing to any peer
			SELECT n_from.id, n_from.file_path, imp.depth + 1
			FROM impact imp
			JOIN nodes n_peer ON n_peer.file_path = imp.file_path
			JOIN edges e ON e.to_id = n_peer.id AND e.valid_until_commit IS NULL
			JOIN nodes n_from ON n_from.id = e.from_id
			WHERE imp.depth < ?
		)
		SELECT DISTINCT n.symbol, n.file_path, n.kind, MIN(imp.depth) AS depth
		FROM impact imp
		JOIN nodes n ON n.id = imp.node_id
		WHERE imp.file_path != ?
		  AND imp.depth > 0
		GROUP BY n.id
	`, filePath, depth, filePath)
	if err != nil {
		return nil, fmt.Errorf("impact query: %w", err)
	}
	defer rows.Close()
	var out []DependentNode
	for rows.Next() {
		var d DependentNode
		if err := rows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.Depth); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Search performs FTS5 search over symbol names and returns up to limit results.
func (q *Querier) Search(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := q.store.db.Query(`
		SELECT n.symbol, n.kind, n.file_path
		FROM nodes_fts f
		JOIN nodes n ON n.id = f.rowid
		WHERE nodes_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer rows.Close()
	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.Symbol, &r.Kind, &r.FilePath); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// NodeInfo returns detailed information for a single node identified by symbol (and optionally kind).
func (q *Querier) NodeInfo(symbol, kind string, includeInvalidated bool) (*NodeInfoResult, error) {
	db := q.store.db
	qStr := `SELECT id, symbol, kind, file_path, COALESCE(line_start,0), COALESCE(line_end,0), language
	         FROM nodes WHERE symbol = ? AND kind != 'external'`
	args := []any{symbol}
	if kind != "" {
		qStr += " AND kind = ?"
		args = append(args, kind)
	}
	qStr += " LIMIT 1"

	var nd NodeDetail
	err := db.QueryRow(qStr, args...).Scan(&nd.ID, &nd.Symbol, &nd.Kind, &nd.FilePath, &nd.LineStart, &nd.LineEnd, &nd.Language)
	if err != nil {
		return nil, fmt.Errorf("node lookup %q: %w", symbol, err)
	}

	// outgoing edges (depends_on)
	outRows, err := db.Query(`
		SELECT n2.symbol, n2.file_path, n2.kind, e.kind
		FROM edges e JOIN nodes n2 ON n2.id = e.to_id
		WHERE e.from_id = ? AND e.valid_until_commit IS NULL`, nd.ID)
	if err != nil {
		return nil, err
	}
	defer outRows.Close()
	var dependsOn []DependentNode
	for outRows.Next() {
		var d DependentNode
		outRows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.EdgeKind)
		dependsOn = append(dependsOn, d)
	}
	outRows.Close()

	// incoming edges (used_by)
	inRows, err := db.Query(`
		SELECT n1.symbol, n1.file_path, n1.kind, e.kind
		FROM edges e JOIN nodes n1 ON n1.id = e.from_id
		WHERE e.to_id = ? AND e.valid_until_commit IS NULL`, nd.ID)
	if err != nil {
		return nil, err
	}
	defer inRows.Close()
	var usedBy []DependentNode
	for inRows.Next() {
		var d DependentNode
		inRows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.EdgeKind)
		usedBy = append(usedBy, d)
	}
	inRows.Close()

	// historical edges (only if requested)
	var historical []DependentNode
	if includeInvalidated {
		hRows, err := db.Query(`
			SELECT n2.symbol, n2.file_path, n2.kind, e.kind
			FROM edges e JOIN nodes n2 ON n2.id = e.to_id
			WHERE e.from_id = ? AND e.valid_until_commit IS NOT NULL`, nd.ID)
		if err != nil {
			return nil, err
		}
		defer hRows.Close()
		for hRows.Next() {
			var d DependentNode
			hRows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.EdgeKind)
			historical = append(historical, d)
		}
	}

	// anchored observation IDs
	aRows, err := db.Query(`SELECT engram_obs_id FROM engram_anchors WHERE node_id = ?`, nd.ID)
	if err != nil {
		return nil, err
	}
	defer aRows.Close()
	var obsIDs []string
	for aRows.Next() {
		var id string
		aRows.Scan(&id)
		obsIDs = append(obsIDs, id)
	}

	return &NodeInfoResult{
		Node:            nd,
		DependsOn:       dependsOn,
		UsedBy:          usedBy,
		HistoricalEdges: historical,
		AnchoredObsIDs:  obsIDs,
	}, nil
}

// AllNodes returns every non-external, non-file node in the graph.
// limit <= 0 returns all nodes.
func (q *Querier) AllNodes(limit int) ([]NodeSummary, error) {
	qStr := `
		SELECT n.symbol, n.kind, n.file_path,
		       count(e.id) AS in_degree
		FROM nodes n
		LEFT JOIN edges e ON e.to_id = n.id AND e.valid_until_commit IS NULL
		WHERE n.kind NOT IN ('external', 'file')
		GROUP BY n.id
		ORDER BY n.symbol
	`
	if limit > 0 {
		qStr += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := q.store.db.Query(qStr)
	if err != nil {
		return nil, fmt.Errorf("all nodes query: %w", err)
	}
	defer rows.Close()
	var out []NodeSummary
	for rows.Next() {
		var ns NodeSummary
		if err := rows.Scan(&ns.Symbol, &ns.Kind, &ns.FilePath, &ns.InDegree); err != nil {
			return nil, err
		}
		out = append(out, ns)
	}
	return out, rows.Err()
}

// Context returns aggregate project statistics for cg_context.
func (q *Querier) Context() (*ProjectContext, error) {
	db := q.store.db

	var total int
	if err := db.QueryRow(`SELECT count(*) FROM nodes WHERE kind != 'external' AND kind != 'file'`).Scan(&total); err != nil {
		return nil, fmt.Errorf("count nodes: %w", err)
	}

	rows, err := db.Query(`
		SELECT language, count(*) FROM nodes
		WHERE kind != 'external' AND language != 'external'
		GROUP BY language
	`)
	if err != nil {
		return nil, fmt.Errorf("languages query: %w", err)
	}
	defer rows.Close()
	langs := []string{}
	counts := map[string]int{}
	for rows.Next() {
		var lang string
		var cnt int
		rows.Scan(&lang, &cnt)
		langs = append(langs, lang)
		counts[lang] = cnt
	}
	rows.Close()

	// Top 10 nodes by incoming edge count.
	topRows, err := db.Query(`
		SELECT n.symbol, n.kind, n.file_path, count(e.id) AS in_degree
		FROM nodes n
		LEFT JOIN edges e ON e.to_id = n.id AND e.valid_until_commit IS NULL
		WHERE n.kind != 'external' AND n.kind != 'file'
		GROUP BY n.id
		ORDER BY in_degree DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, fmt.Errorf("top nodes query: %w", err)
	}
	defer topRows.Close()
	var top []NodeSummary
	for topRows.Next() {
		var ns NodeSummary
		topRows.Scan(&ns.Symbol, &ns.Kind, &ns.FilePath, &ns.InDegree)
		top = append(top, ns)
	}

	return &ProjectContext{
		TotalNodes: total,
		Languages:  langs,
		TopNodes:   top,
		NodeCounts: counts,
	}, nil
}
