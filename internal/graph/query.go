package graph

import "fmt"

// DependentNode is a node returned by dependency queries.
type DependentNode struct {
	Symbol   string `json:"symbol"`
	FilePath string `json:"file_path"`
	Kind     string `json:"kind"`
	EdgeKind string `json:"edge_kind"`
	Depth    int    `json:"depth"`
	Root     string `json:"root"`
}

// SearchResult is a node returned by FTS5 symbol search.
type SearchResult struct {
	Symbol   string `json:"symbol"`
	Kind     string `json:"kind"`
	FilePath string `json:"file_path"`
	Root     string `json:"root"`
}

// NodeSummary is a compact node representation for project context.
type NodeSummary struct {
	Symbol   string `json:"symbol"`
	Kind     string `json:"kind"`
	FilePath string `json:"file_path"`
	InDegree int    `json:"in_degree"`
	Root     string `json:"root"`
}

// RootContext is per-root statistics returned by Context.
type RootContext struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Remote     string   `json:"remote"`
	Branch     string   `json:"branch"`
	VCS        string   `json:"vcs"`
	TotalNodes int      `json:"total_nodes"`
	Languages  []string `json:"languages"`
}

// ProjectContext is the high-level summary returned by cg_context.
type ProjectContext struct {
	TotalNodes int               `json:"total_nodes"`
	Languages  []string          `json:"languages"`
	TopNodes   []NodeSummary     `json:"top_nodes"`
	NodeCounts map[string]int    `json:"node_counts"`
	Roots      []RootContext     `json:"roots"`
}

// NodeDetail holds the full metadata for a single graph node.
type NodeDetail struct {
	ID        int64  `json:"id"`
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind"`
	FilePath  string `json:"file_path"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Language  string `json:"language"`
	Root      string `json:"root"`
}

// NodeInfoResult holds a node's details plus its incoming/outgoing edges and anchors.
type NodeInfoResult struct {
	Node            NodeDetail      `json:"node"`
	DependsOn       []DependentNode `json:"depends_on"`
	UsedBy          []DependentNode `json:"used_by"`
	HistoricalEdges []DependentNode `json:"historical_edges"`
	AnchoredObsIDs  []string        `json:"anchored_obs_ids"`
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
// If rootName != "", only considers nodes within that root.
func (q *Querier) Dependencies(filePath, rootName string) ([]DependentNode, error) {
	qStr := `
		SELECT n2.symbol, n2.file_path, n2.kind, e.kind, COALESCE(r2.name,'')
		FROM edges e
		JOIN nodes n1 ON n1.id = e.from_id
		JOIN nodes n2 ON n2.id = e.to_id
		JOIN roots r1 ON r1.id = n1.root_id
		JOIN roots r2 ON r2.id = n2.root_id
		WHERE n1.file_path = ?
		  AND e.valid_until_rev IS NULL`
	args := []any{filePath}
	if rootName != "" {
		qStr += " AND r1.name = ?"
		args = append(args, rootName)
	}
	rows, err := q.store.db.Query(qStr, args...)
	if err != nil {
		return nil, fmt.Errorf("dependencies query: %w", err)
	}
	defer rows.Close()
	var out []DependentNode
	for rows.Next() {
		var d DependentNode
		if err := rows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.EdgeKind, &d.Root); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Dependents returns all nodes that have active edges pointing into nodes of filePath.
// If rootName != "", only considers the target file within that root.
func (q *Querier) Dependents(filePath, rootName string) ([]DependentNode, error) {
	qStr := `
		SELECT DISTINCT n1.symbol, n1.file_path, n1.kind, e.kind, COALESCE(r1.name,'')
		FROM edges e
		JOIN nodes n1 ON n1.id = e.from_id
		JOIN nodes n2 ON n2.id = e.to_id
		JOIN roots r1 ON r1.id = n1.root_id
		JOIN roots r2 ON r2.id = n2.root_id
		WHERE n2.file_path = ?
		  AND e.valid_until_rev IS NULL`
	args := []any{filePath}
	if rootName != "" {
		qStr += " AND r2.name = ?"
		args = append(args, rootName)
	}
	rows, err := q.store.db.Query(qStr, args...)
	if err != nil {
		return nil, fmt.Errorf("dependents query: %w", err)
	}
	defer rows.Close()
	var out []DependentNode
	for rows.Next() {
		var d DependentNode
		if err := rows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.EdgeKind, &d.Root); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Impact returns the transitive blast radius of modifying filePath up to depth hops.
// If rootName != "", only starts from nodes within that root.
func (q *Querier) Impact(filePath string, depth int, rootName string) ([]DependentNode, error) {
	if depth <= 0 {
		depth = 3
	}

	baseArgs := []any{filePath, depth, filePath}
	rootFilter := ""
	if rootName != "" {
		rootFilter = " AND r_seed.name = ?"
		baseArgs = []any{filePath, rootName, depth, filePath}
	}

	qStr := fmt.Sprintf(`
		WITH RECURSIVE impact(node_id, file_path, depth) AS (
			SELECT n.id, n.file_path, 0
			FROM nodes n
			JOIN roots r_seed ON r_seed.id = n.root_id
			WHERE n.file_path = ?%s
			UNION
			SELECT n_from.id, n_from.file_path, imp.depth + 1
			FROM impact imp
			JOIN nodes n_peer ON n_peer.file_path = imp.file_path
			JOIN edges e ON e.to_id = n_peer.id AND e.valid_until_rev IS NULL
			JOIN nodes n_from ON n_from.id = e.from_id
			WHERE imp.depth < ?
		)
		SELECT DISTINCT n.symbol, n.file_path, n.kind, MIN(imp.depth) AS depth, COALESCE(r.name,'')
		FROM impact imp
		JOIN nodes n ON n.id = imp.node_id
		JOIN roots r ON r.id = n.root_id
		WHERE imp.file_path != ?
		  AND imp.depth > 0
		GROUP BY n.id`, rootFilter)

	rows, err := q.store.db.Query(qStr, baseArgs...)
	if err != nil {
		return nil, fmt.Errorf("impact query: %w", err)
	}
	defer rows.Close()
	var out []DependentNode
	for rows.Next() {
		var d DependentNode
		if err := rows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.Depth, &d.Root); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Search performs FTS5 search over symbol names and returns up to limit results.
// If rootName != "", only returns results from that root.
func (q *Querier) Search(query string, limit int, rootName string) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	qStr := `
		SELECT n.symbol, n.kind, n.file_path, COALESCE(r.name,'')
		FROM nodes_fts f
		JOIN nodes n ON n.id = f.rowid
		JOIN roots r ON r.id = n.root_id
		WHERE nodes_fts MATCH ?`
	args := []any{query}
	if rootName != "" {
		qStr += " AND r.name = ?"
		args = append(args, rootName)
	}
	qStr += " ORDER BY rank LIMIT ?"
	args = append(args, limit)

	rows, err := q.store.db.Query(qStr, args...)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer rows.Close()
	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.Symbol, &r.Kind, &r.FilePath, &r.Root); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// NodeInfo returns detailed information for a single node identified by symbol (and optionally kind).
// If rootName != "", restricts to that root.
func (q *Querier) NodeInfo(symbol, kind string, includeInvalidated bool, rootName string) (*NodeInfoResult, error) {
	db := q.store.db
	qStr := `
		SELECT n.id, n.symbol, n.kind, n.file_path,
		       COALESCE(n.line_start,0), COALESCE(n.line_end,0), n.language, COALESCE(r.name,'')
		FROM nodes n
		JOIN roots r ON r.id = n.root_id
		WHERE n.symbol = ? AND n.kind != 'external'`
	args := []any{symbol}
	if kind != "" {
		qStr += " AND n.kind = ?"
		args = append(args, kind)
	}
	if rootName != "" {
		qStr += " AND r.name = ?"
		args = append(args, rootName)
	}
	qStr += " LIMIT 1"

	var nd NodeDetail
	err := db.QueryRow(qStr, args...).Scan(
		&nd.ID, &nd.Symbol, &nd.Kind, &nd.FilePath,
		&nd.LineStart, &nd.LineEnd, &nd.Language, &nd.Root,
	)
	if err != nil {
		return nil, fmt.Errorf("node lookup %q: %w", symbol, err)
	}

	outRows, err := db.Query(`
		SELECT n2.symbol, n2.file_path, n2.kind, e.kind, COALESCE(r2.name,'')
		FROM edges e
		JOIN nodes n2 ON n2.id = e.to_id
		JOIN roots r2 ON r2.id = n2.root_id
		WHERE e.from_id = ? AND e.valid_until_rev IS NULL`, nd.ID)
	if err != nil {
		return nil, err
	}
	defer outRows.Close()
	var dependsOn []DependentNode
	for outRows.Next() {
		var d DependentNode
		outRows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.EdgeKind, &d.Root)
		dependsOn = append(dependsOn, d)
	}
	outRows.Close()

	inRows, err := db.Query(`
		SELECT n1.symbol, n1.file_path, n1.kind, e.kind, COALESCE(r1.name,'')
		FROM edges e
		JOIN nodes n1 ON n1.id = e.from_id
		JOIN roots r1 ON r1.id = n1.root_id
		WHERE e.to_id = ? AND e.valid_until_rev IS NULL`, nd.ID)
	if err != nil {
		return nil, err
	}
	defer inRows.Close()
	var usedBy []DependentNode
	for inRows.Next() {
		var d DependentNode
		inRows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.EdgeKind, &d.Root)
		usedBy = append(usedBy, d)
	}
	inRows.Close()

	var historical []DependentNode
	if includeInvalidated {
		hRows, err := db.Query(`
			SELECT n2.symbol, n2.file_path, n2.kind, e.kind, COALESCE(r2.name,'')
			FROM edges e
			JOIN nodes n2 ON n2.id = e.to_id
			JOIN roots r2 ON r2.id = n2.root_id
			WHERE e.from_id = ? AND e.valid_until_rev IS NOT NULL`, nd.ID)
		if err != nil {
			return nil, err
		}
		defer hRows.Close()
		for hRows.Next() {
			var d DependentNode
			hRows.Scan(&d.Symbol, &d.FilePath, &d.Kind, &d.EdgeKind, &d.Root)
			historical = append(historical, d)
		}
	}

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

// AllNodes returns every non-external, non-file node in the graph with its root name.
// limit <= 0 returns all nodes.
func (q *Querier) AllNodes(limit int) ([]NodeSummary, error) {
	qStr := `
		SELECT n.symbol, n.kind, n.file_path,
		       COUNT(e.id) AS in_degree, COALESCE(r.name,'')
		FROM nodes n
		JOIN roots r ON r.id = n.root_id
		LEFT JOIN edges e ON e.to_id = n.id AND e.valid_until_rev IS NULL
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
		if err := rows.Scan(&ns.Symbol, &ns.Kind, &ns.FilePath, &ns.InDegree, &ns.Root); err != nil {
			return nil, err
		}
		out = append(out, ns)
	}
	return out, rows.Err()
}

// GraphNode is a node returned by the graph visualization endpoint.
type GraphNode struct {
	ID       int64  `json:"id"`
	Symbol   string `json:"symbol"`
	Kind     string `json:"kind"`
	FilePath string `json:"file_path"`
	Root     string `json:"root"`
}

// GraphEdge is an active edge returned by the graph visualization endpoint.
type GraphEdge struct {
	From int64  `json:"from"`
	To   int64  `json:"to"`
	Kind string `json:"kind"`
}

// GraphDataResult is the shape returned by /api/graph.
type GraphDataResult struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphData returns all non-external nodes and their active edges for the force-directed canvas.
// If rootName != "", only returns nodes and edges belonging to that root.
func (q *Querier) GraphData(rootName string) (*GraphDataResult, error) {
	nodeQ := `
		SELECT n.id, n.symbol, n.kind, n.file_path, COALESCE(r.name,'')
		FROM nodes n
		JOIN roots r ON r.id = n.root_id
		WHERE n.kind != 'external'`
	nodeArgs := []any{}
	if rootName != "" {
		nodeQ += " AND r.name = ?"
		nodeArgs = append(nodeArgs, rootName)
	}
	nodeQ += " ORDER BY n.id"

	nRows, err := q.store.db.Query(nodeQ, nodeArgs...)
	if err != nil {
		return nil, fmt.Errorf("graph nodes query: %w", err)
	}
	defer nRows.Close()
	var nodes []GraphNode
	nodeSet := map[int64]bool{}
	for nRows.Next() {
		var gn GraphNode
		if err := nRows.Scan(&gn.ID, &gn.Symbol, &gn.Kind, &gn.FilePath, &gn.Root); err != nil {
			return nil, err
		}
		nodes = append(nodes, gn)
		nodeSet[gn.ID] = true
	}
	if err := nRows.Err(); err != nil {
		return nil, err
	}

	edgeQ := `
		SELECT e.from_id, e.to_id, e.kind
		FROM edges e
		JOIN nodes fn ON fn.id = e.from_id
		JOIN roots fr ON fr.id = fn.root_id
		JOIN nodes tn ON tn.id = e.to_id
		WHERE e.valid_until_rev IS NULL
		  AND fn.kind != 'external'
		  AND tn.kind != 'external'`
	edgeArgs := []any{}
	if rootName != "" {
		edgeQ += " AND fr.name = ?"
		edgeArgs = append(edgeArgs, rootName)
	}

	eRows, err := q.store.db.Query(edgeQ, edgeArgs...)
	if err != nil {
		return nil, fmt.Errorf("graph edges query: %w", err)
	}
	defer eRows.Close()
	var edges []GraphEdge
	for eRows.Next() {
		var ge GraphEdge
		if err := eRows.Scan(&ge.From, &ge.To, &ge.Kind); err != nil {
			return nil, err
		}
		// only emit edge if both endpoints are in the visible node set
		if nodeSet[ge.From] && nodeSet[ge.To] {
			edges = append(edges, ge)
		}
	}
	if err := eRows.Err(); err != nil {
		return nil, err
	}

	if nodes == nil {
		nodes = []GraphNode{}
	}
	if edges == nil {
		edges = []GraphEdge{}
	}
	return &GraphDataResult{Nodes: nodes, Edges: edges}, nil
}

// Context returns aggregate project statistics for cg_context, including per-root breakdown.
func (q *Querier) Context() (*ProjectContext, error) {
	db := q.store.db

	var total int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM nodes WHERE kind != 'external' AND kind != 'file'`,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count nodes: %w", err)
	}

	rows, err := db.Query(`
		SELECT language, COUNT(*) FROM nodes
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

	topRows, err := db.Query(`
		SELECT n.symbol, n.kind, n.file_path, COUNT(e.id) AS in_degree, COALESCE(r.name,'')
		FROM nodes n
		JOIN roots r ON r.id = n.root_id
		LEFT JOIN edges e ON e.to_id = n.id AND e.valid_until_rev IS NULL
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
		topRows.Scan(&ns.Symbol, &ns.Kind, &ns.FilePath, &ns.InDegree, &ns.Root)
		top = append(top, ns)
	}

	// Per-root context.
	rootRows, err := db.Query(`
		SELECT r.name, r.rel_path, COALESCE(r.remote_url,''), COALESCE(r.default_branch,''), r.vcs,
		       COUNT(DISTINCT n.id)
		FROM roots r
		LEFT JOIN nodes n ON n.root_id = r.id AND n.kind != 'external' AND n.kind != 'file'
		GROUP BY r.id
		ORDER BY r.name
	`)
	if err != nil {
		return nil, fmt.Errorf("roots context query: %w", err)
	}
	defer rootRows.Close()
	var rootCtxs []RootContext
	for rootRows.Next() {
		var rc RootContext
		rootRows.Scan(&rc.Name, &rc.Path, &rc.Remote, &rc.Branch, &rc.VCS, &rc.TotalNodes)
		rootCtxs = append(rootCtxs, rc)
	}
	rootRows.Close()

	// Lenguajes por raíz.
	for i, rc := range rootCtxs {
		langRows, err := db.Query(`
			SELECT DISTINCT n.language
			FROM nodes n
			JOIN roots r ON r.id = n.root_id
			WHERE r.name = ? AND n.kind != 'external' AND n.language != 'external'
		`, rc.Name)
		if err != nil {
			continue
		}
		var rootLangs []string
		for langRows.Next() {
			var l string
			langRows.Scan(&l)
			rootLangs = append(rootLangs, l)
		}
		langRows.Close()
		rootCtxs[i].Languages = rootLangs
	}

	return &ProjectContext{
		TotalNodes: total,
		Languages:  langs,
		TopNodes:   top,
		NodeCounts: counts,
		Roots:      rootCtxs,
	}, nil
}
