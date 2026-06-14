package graph

import "fmt"

// HistoryEdgeEvent represents a single edge appearance or disappearance in the node's timeline.
type HistoryEdgeEvent struct {
	Commit         string // git commit hash (empty when revision has no commit_hash)
	EventType      string // "appeared" | "disappeared"
	TargetSymbol   string
	TargetFilePath string
	TargetKind     string
	EdgeKind       string
}

// HistoryResult holds the full edge timeline for a single node.
type HistoryResult struct {
	Symbol         string
	Kind           string
	FilePath       string
	Language       string
	Timeline       []HistoryEdgeEvent
	AnchoredObsIDs []string
}

// History returns the chronological edge timeline for the node identified by symbol and kind.
// The timeline includes edge appearances (valid_from_rev) and disappearances (valid_until_rev)
// ordered by revision ID (monotonic).
func (q *Querier) History(symbol, kind string) (*HistoryResult, error) {
	db := q.store.db

	qStr := `SELECT id, symbol, kind, file_path, language
	         FROM nodes WHERE symbol = ? AND kind != 'external'`
	args := []any{symbol}
	if kind != "" {
		qStr += " AND kind = ?"
		args = append(args, kind)
	}
	qStr += " LIMIT 1"

	var nd NodeDetail
	err := db.QueryRow(qStr, args...).Scan(&nd.ID, &nd.Symbol, &nd.Kind, &nd.FilePath, &nd.Language)
	if err != nil {
		return nil, fmt.Errorf("node lookup %q: %w", symbol, err)
	}

	// Edges are stored FROM the file node; resolve it.
	var fileNodeID int64
	err = db.QueryRow(
		`SELECT id FROM nodes WHERE symbol = ? AND kind = 'file' LIMIT 1`, nd.FilePath,
	).Scan(&fileNodeID)
	if err != nil {
		return nil, fmt.Errorf("file node lookup for %q: %w", nd.FilePath, err)
	}

	// Timeline: union of appearances and disappearances, ordered by revision ID (monotonic).
	timelineRows, err := db.Query(`
		SELECT event_type, commit_hash, target_symbol, target_file_path, target_kind, edge_kind
		FROM (
			SELECT
				'appeared'                    AS event_type,
				COALESCE(r.commit_hash, '')   AS commit_hash,
				n2.symbol                     AS target_symbol,
				n2.file_path                  AS target_file_path,
				n2.kind                       AS target_kind,
				e.kind                        AS edge_kind,
				r.id                          AS sort_rev
			FROM edges e
			JOIN nodes n2 ON n2.id = e.to_id
			JOIN revisions r ON r.id = e.valid_from_rev
			WHERE e.from_id = ?

			UNION ALL

			SELECT
				'disappeared'                 AS event_type,
				COALESCE(r.commit_hash, '')   AS commit_hash,
				n2.symbol                     AS target_symbol,
				n2.file_path                  AS target_file_path,
				n2.kind                       AS target_kind,
				e.kind                        AS edge_kind,
				r.id                          AS sort_rev
			FROM edges e
			JOIN nodes n2 ON n2.id = e.to_id
			JOIN revisions r ON r.id = e.valid_until_rev
			WHERE e.from_id = ?
			  AND e.valid_until_rev IS NOT NULL
		)
		ORDER BY sort_rev, event_type
	`, fileNodeID, fileNodeID)
	if err != nil {
		return nil, fmt.Errorf("timeline query: %w", err)
	}
	defer timelineRows.Close()

	var timeline []HistoryEdgeEvent
	for timelineRows.Next() {
		var ev HistoryEdgeEvent
		if err := timelineRows.Scan(
			&ev.EventType, &ev.Commit,
			&ev.TargetSymbol, &ev.TargetFilePath, &ev.TargetKind, &ev.EdgeKind,
		); err != nil {
			return nil, err
		}
		timeline = append(timeline, ev)
	}
	if err := timelineRows.Err(); err != nil {
		return nil, err
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

	return &HistoryResult{
		Symbol:         nd.Symbol,
		Kind:           nd.Kind,
		FilePath:       nd.FilePath,
		Language:       nd.Language,
		Timeline:       timeline,
		AnchoredObsIDs: obsIDs,
	}, nil
}
