package graph

import "fmt"

// OrphanNode is a node that never had any incoming edges.
type OrphanNode struct {
	Symbol   string
	Kind     string
	FilePath string
	Language string
	Root     string
}

// AbandonedNode is a node that once had incoming edges but now has none.
// DaysSinceAbandoned is exact: computed from the created_at of the invalidating revision.
type AbandonedNode struct {
	Symbol             string
	Kind               string
	FilePath           string
	Language           string
	PeakIncomingEdges  int
	DaysSinceAbandoned float64
	Root               string
}

// DeadcodeResult holds the output of a dead-code scan.
type DeadcodeResult struct {
	Orphans   []OrphanNode
	Abandoned []AbandonedNode
}

// Deadcode scans the graph for dead-code candidates.
//
// Orphans: nodes that never had any incoming edge (all-time zero references).
// Abandoned: nodes that once had incoming edges but have none active now.
//
// Filters applied automatically:
//   - kind IN (external, file, package) excluded
//   - test files (*_test.go) excluded
//   - exported Go symbols (uppercase first char) excluded
//   - symbol "main" excluded
//
// thresholdDays: skip nodes whose most recent edge activity is less than
// thresholdDays old. 0 means no filter.
func (q *Querier) Deadcode(thresholdDays int) (*DeadcodeResult, error) {
	orphans, err := q.queryOrphans(thresholdDays)
	if err != nil {
		return nil, fmt.Errorf("orphans: %w", err)
	}
	abandoned, err := q.queryAbandoned(thresholdDays)
	if err != nil {
		return nil, fmt.Errorf("abandoned: %w", err)
	}
	return &DeadcodeResult{Orphans: orphans, Abandoned: abandoned}, nil
}

// deadcodeFilter is the shared WHERE clause for both queries.
const deadcodeFilter = `
    n.kind NOT IN ('external', 'file', 'package')
    AND n.file_path NOT LIKE '%_test.go'
    AND NOT (n.language = 'go' AND n.symbol GLOB '[A-Z]*')
    AND n.symbol != 'main'
`

func (q *Querier) queryOrphans(thresholdDays int) ([]OrphanNode, error) {
	sqlStr := `
		SELECT n.symbol, n.kind, n.file_path, n.language, COALESCE(r.name,'')
		FROM nodes n
		JOIN roots r ON r.id = n.root_id
		WHERE ` + deadcodeFilter + `
		  AND NOT EXISTS (
		      SELECT 1 FROM edges e WHERE e.to_id = n.id
		  )
	`
	if thresholdDays > 0 {
		sqlStr += fmt.Sprintf(`
		  AND julianday('now') - julianday(n.created_at) > %d`, thresholdDays)
	}

	rows, err := q.store.db.Query(sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OrphanNode
	for rows.Next() {
		var o OrphanNode
		if err := rows.Scan(&o.Symbol, &o.Kind, &o.FilePath, &o.Language, &o.Root); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (q *Querier) queryAbandoned(thresholdDays int) ([]AbandonedNode, error) {
	// days_since_abandoned: exact from the created_at of the invalidating revision.
	sqlStr := `
		SELECT n.symbol, n.kind, n.file_path, n.language,
		       COUNT(e_hist.id) AS peak_incoming_edges,
		       COALESCE(
		           julianday('now') - julianday(MAX(r.created_at)),
		           0
		       ) AS days_since_abandoned,
		       COALESCE(r_root.name,'')
		FROM nodes n
		JOIN roots r_root ON r_root.id = n.root_id
		JOIN edges e_hist ON e_hist.to_id = n.id AND e_hist.valid_until_rev IS NOT NULL
		JOIN revisions r ON r.id = e_hist.valid_until_rev
		WHERE ` + deadcodeFilter + `
		  AND NOT EXISTS (
		      SELECT 1 FROM edges e WHERE e.to_id = n.id AND e.valid_until_rev IS NULL
		  )
	`
	if thresholdDays > 0 {
		sqlStr += fmt.Sprintf(`
		  AND (julianday('now') - julianday(MAX(r.created_at))) > %d`, thresholdDays)
	}
	sqlStr += `
		GROUP BY n.id
	`

	rows, err := q.store.db.Query(sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AbandonedNode
	for rows.Next() {
		var a AbandonedNode
		if err := rows.Scan(&a.Symbol, &a.Kind, &a.FilePath, &a.Language,
			&a.PeakIncomingEdges, &a.DaysSinceAbandoned, &a.Root); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
