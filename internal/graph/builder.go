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
// rootID scopes the nodes to a specific indexed root.
// revID is the revision at which this version of the file is observed.
// fileChecksum is the sha256 hex of the file's content (stored on the file node).
// Bi-temporal: edges removed from the new result are invalidated (not deleted).
func (b *Builder) UpsertFile(rootID, revID int64, fileChecksum string, result *parser.Result) error {
	db := b.store.db

	lang := "unknown"
	for _, n := range result.Nodes {
		if n.Language != "" {
			lang = n.Language
			break
		}
	}

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

	// Always upsert a file-level node; edges originate from it.
	fileNodeID, err := upsertNode(tx, rootID, filePath, "file", filePath, 0, 0, lang, fileChecksum)
	if err != nil {
		return fmt.Errorf("upsert file node: %w", err)
	}

	for _, n := range result.Nodes {
		if _, err := upsertNode(tx, rootID, n.Symbol, n.Kind, n.FilePath, n.LineStart, n.LineEnd, n.Language, ""); err != nil {
			return fmt.Errorf("upsert node %s: %w", n.Symbol, err)
		}
	}

	type edgeKey struct{ toSymbol, kind string }
	wantEdges := make(map[edgeKey]struct{}, len(result.Edges))
	for _, e := range result.Edges {
		wantEdges[edgeKey{e.ToSymbol, e.Kind}] = struct{}{}
	}

	activeEdges, err := loadActiveEdgesFrom(tx, fileNodeID)
	if err != nil {
		return fmt.Errorf("load active edges: %w", err)
	}

	for _, ae := range activeEdges {
		if _, keep := wantEdges[edgeKey{ae.toSymbol, ae.kind}]; !keep {
			if err := invalidateEdge(tx, ae.id, revID); err != nil {
				return fmt.Errorf("invalidate edge %d: %w", ae.id, err)
			}
		}
	}

	activeSet := make(map[edgeKey]struct{}, len(activeEdges))
	for _, ae := range activeEdges {
		activeSet[edgeKey{ae.toSymbol, ae.kind}] = struct{}{}
	}

	for _, e := range result.Edges {
		ek := edgeKey{e.ToSymbol, e.Kind}
		if _, exists := activeSet[ek]; exists {
			continue
		}
		toID, err := resolveOrCreateNode(tx, rootID, e.ToSymbol)
		if err != nil {
			return fmt.Errorf("resolve target %s: %w", e.ToSymbol, err)
		}
		if err := insertEdge(tx, fileNodeID, toID, e.Kind, revID); err != nil {
			return fmt.Errorf("insert edge %s→%s: %w", filePath, e.ToSymbol, err)
		}
	}

	return tx.Commit()
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
func upsertNode(tx *sql.Tx, rootID int64, symbol, kind, filePath string, lineStart, lineEnd int, language, checksum string) (int64, error) {
	var csNullable interface{}
	if checksum != "" {
		csNullable = checksum
	}
	_, err := tx.Exec(`
		INSERT INTO nodes(root_id, symbol, kind, file_path, line_start, line_end, language, checksum)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(root_id, symbol, file_path, kind) DO UPDATE SET
			line_start = excluded.line_start,
			line_end   = excluded.line_end,
			checksum   = excluded.checksum,
			updated_at = CURRENT_TIMESTAMP
	`, rootID, symbol, kind, filePath, lineStart, lineEnd, language, csNullable)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRow(
		`SELECT id FROM nodes WHERE root_id=? AND symbol=? AND file_path=? AND kind=?`,
		rootID, symbol, filePath, kind,
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
		WHERE e.from_id = ? AND e.valid_until_rev IS NULL
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

// invalidateEdge sets valid_until_rev on the given edge.
func invalidateEdge(tx *sql.Tx, edgeID, revID int64) error {
	_, err := tx.Exec(
		`UPDATE edges SET valid_until_rev=? WHERE id=?`,
		revID, edgeID,
	)
	return err
}

// resolveOrCreateNode finds an existing non-external node by symbol in the given root,
// or creates an external stub scoped to that root.
func resolveOrCreateNode(tx *sql.Tx, rootID int64, symbol string) (int64, error) {
	var id int64
	err := tx.QueryRow(
		`SELECT id FROM nodes WHERE root_id=? AND symbol=? AND kind!='external' LIMIT 1`,
		rootID, symbol,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	return upsertNode(tx, rootID, symbol, "external", ":external:", 0, 0, "external", "")
}

// insertEdge creates a new active edge.
func insertEdge(tx *sql.Tx, fromID, toID int64, kind string, revID int64) error {
	_, err := tx.Exec(`
		INSERT INTO edges(from_id, to_id, kind, valid_from_rev)
		VALUES(?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, fromID, toID, kind, revID)
	return err
}
