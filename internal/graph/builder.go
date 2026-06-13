package graph

import (
	"database/sql"
	"fmt"

	"github.com/Jomruizgo/Engrafo/internal/parser"
)

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
func (b *Builder) UpsertFile(commitHash string, result *parser.Result) error {
	db := b.store.db

	// Determine the primary language from the first node (all nodes in a file share language).
	lang := "unknown"
	for _, n := range result.Nodes {
		if n.Language != "" {
			lang = n.Language
			break
		}
	}

	// Derive the file path from the first node, or from edges.
	filePath := ""
	for _, n := range result.Nodes {
		if n.FilePath != "" {
			filePath = n.FilePath
			break
		}
	}
	if filePath == "" {
		for _, e := range result.Edges {
			if e.FromSymbol != "" {
				filePath = e.FromSymbol
				break
			}
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Always upsert a file-level node so that edges can use it as "from".
	fileNodeID, err := upsertNode(tx, filePath, "file", filePath, 0, 0, lang)
	if err != nil {
		return fmt.Errorf("upsert file node: %w", err)
	}

	// Upsert all explicit symbol nodes.
	for _, n := range result.Nodes {
		if _, err := upsertNode(tx, n.Symbol, n.Kind, n.FilePath, n.LineStart, n.LineEnd, n.Language); err != nil {
			return fmt.Errorf("upsert node %s: %w", n.Symbol, err)
		}
	}

	// Compute the set of (toSymbol, edgeKind) pairs that should be active after this upsert.
	type edgeKey struct{ toSymbol, kind string }
	wantEdges := make(map[edgeKey]struct{}, len(result.Edges))
	for _, e := range result.Edges {
		wantEdges[edgeKey{e.ToSymbol, e.Kind}] = struct{}{}
	}

	// Load active edges that currently originate from the file node.
	activeEdges, err := loadActiveEdgesFrom(tx, fileNodeID)
	if err != nil {
		return fmt.Errorf("load active edges: %w", err)
	}

	// Invalidate edges that are no longer in the new result.
	for _, ae := range activeEdges {
		if _, keep := wantEdges[edgeKey{ae.toSymbol, ae.kind}]; !keep {
			if err := invalidateEdge(tx, ae.id, commitHash); err != nil {
				return fmt.Errorf("invalidate edge %d: %w", ae.id, err)
			}
		}
	}

	// Build set of already-active toSymbols to avoid re-inserting.
	activeSet := make(map[edgeKey]struct{}, len(activeEdges))
	for _, ae := range activeEdges {
		activeSet[edgeKey{ae.toSymbol, ae.kind}] = struct{}{}
	}

	// Insert new edges.
	for _, e := range result.Edges {
		ek := edgeKey{e.ToSymbol, e.Kind}
		if _, exists := activeSet[ek]; exists {
			continue // already active
		}
		toID, err := resolveOrCreateNode(tx, e.ToSymbol)
		if err != nil {
			return fmt.Errorf("resolve target %s: %w", e.ToSymbol, err)
		}
		if err := insertEdge(tx, fileNodeID, toID, e.Kind, commitHash); err != nil {
			return fmt.Errorf("insert edge %s→%s: %w", filePath, e.ToSymbol, err)
		}
	}

	return tx.Commit()
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

// ---- helpers ----

// upsertNode inserts or updates a node and returns its ID.
// Always SELECT after UPSERT — LastInsertId is unreliable for ON CONFLICT DO UPDATE.
func upsertNode(tx *sql.Tx, symbol, kind, filePath string, lineStart, lineEnd int, language string) (int64, error) {
	_, err := tx.Exec(`
		INSERT INTO nodes(symbol, kind, file_path, line_start, line_end, language)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(symbol, file_path, kind) DO UPDATE SET
			line_start = excluded.line_start,
			line_end   = excluded.line_end,
			updated_at = CURRENT_TIMESTAMP
	`, symbol, kind, filePath, lineStart, lineEnd, language)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRow(
		`SELECT id FROM nodes WHERE symbol=? AND file_path=? AND kind=?`,
		symbol, filePath, kind,
	).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

type activeEdge struct {
	id       int64
	toSymbol string
	kind     string
}

// loadActiveEdgesFrom returns active edges whose from_id = nodeID.
func loadActiveEdgesFrom(tx *sql.Tx, nodeID int64) ([]activeEdge, error) {
	rows, err := tx.Query(`
		SELECT e.id, n.symbol, e.kind
		FROM edges e
		JOIN nodes n ON n.id = e.to_id
		WHERE e.from_id = ? AND e.valid_until_commit IS NULL
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []activeEdge
	for rows.Next() {
		var ae activeEdge
		if err := rows.Scan(&ae.id, &ae.toSymbol, &ae.kind); err != nil {
			return nil, err
		}
		out = append(out, ae)
	}
	return out, rows.Err()
}

// invalidateEdge sets valid_until_commit on edge id.
func invalidateEdge(tx *sql.Tx, edgeID int64, commitHash string) error {
	_, err := tx.Exec(
		`UPDATE edges SET valid_until_commit=? WHERE id=?`,
		commitHash, edgeID,
	)
	return err
}

// resolveOrCreateNode finds an existing node by symbol name, or creates an external stub.
func resolveOrCreateNode(tx *sql.Tx, symbol string) (int64, error) {
	var id int64
	err := tx.QueryRow(
		`SELECT id FROM nodes WHERE symbol=? AND kind != 'external' LIMIT 1`,
		symbol,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	// Create external stub node.
	return upsertNode(tx, symbol, "external", ":external:", 0, 0, "external")
}

// insertEdge creates a new active edge.
func insertEdge(tx *sql.Tx, fromID, toID int64, kind, commitHash string) error {
	_, err := tx.Exec(`
		INSERT INTO edges(from_id, to_id, kind, valid_from_commit)
		VALUES(?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, fromID, toID, kind, commitHash)
	return err
}
