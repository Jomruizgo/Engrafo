package graph

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/Jomruizgo/Engrafo/v2/internal/parser"
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

	// Always upsert a file-level node; edges default to originating from it.
	fileNodeID, err := upsertNode(tx, rootID, filePath, "file", filePath, 0, 0, lang, fileChecksum)
	if err != nil {
		return fmt.Errorf("upsert file node: %w", err)
	}

	// localNodes maps a symbol defined in THIS file to its node id, so that
	// edge endpoints resolve within the same file first. This lets extractors
	// emit symbol->symbol edges (e.g. CloudFormation resource->resource) instead
	// of only file->symbol, without cross-file symbol-name ambiguity.
	localNodes := map[string]int64{filePath: fileNodeID}
	for _, n := range result.Nodes {
		id, err := upsertNode(tx, rootID, n.Symbol, n.Kind, n.FilePath, n.LineStart, n.LineEnd, n.Language, "")
		if err != nil {
			return fmt.Errorf("upsert node %s: %w", n.Symbol, err)
		}
		localNodes[n.Symbol] = id
	}

	// resolveFrom: an edge's origin. When FromSymbol is empty or equals the file
	// path it is the file node (the TS/Py/Go model). When it names a symbol defined
	// in this file, the edge originates from that symbol node (the CFN model).
	resolveFrom := func(fromSymbol string) int64 {
		if fromSymbol == "" || fromSymbol == filePath {
			return fileNodeID
		}
		if id, ok := localNodes[fromSymbol]; ok {
			return id
		}
		return fileNodeID
	}

	type edgeKey struct{ fromSymbol, toSymbol, kind string }
	wantEdges := make(map[edgeKey]struct{}, len(result.Edges))
	for _, e := range result.Edges {
		wantEdges[edgeKey{e.FromSymbol, e.ToSymbol, e.Kind}] = struct{}{}
	}

	// Load active edges from every node belonging to this file (file node + symbols),
	// so bi-temporal invalidation covers symbol->symbol edges too.
	activeEdges, err := loadActiveEdgesForFile(tx, rootID, filePath)
	if err != nil {
		return fmt.Errorf("load active edges: %w", err)
	}

	for _, ae := range activeEdges {
		if _, keep := wantEdges[edgeKey{ae.fromSymbol, ae.toSymbol, ae.kind}]; !keep {
			if err := invalidateEdge(tx, ae.id, revID); err != nil {
				return fmt.Errorf("invalidate edge %d: %w", ae.id, err)
			}
		}
	}

	activeSet := make(map[edgeKey]struct{}, len(activeEdges))
	for _, ae := range activeEdges {
		activeSet[edgeKey{ae.fromSymbol, ae.toSymbol, ae.kind}] = struct{}{}
	}

	for _, e := range result.Edges {
		ek := edgeKey{e.FromSymbol, e.ToSymbol, e.Kind}
		if _, exists := activeSet[ek]; exists {
			continue
		}
		fromID := resolveFrom(e.FromSymbol)
		var toID int64
		if id, ok := localNodes[e.ToSymbol]; ok {
			toID = id
		} else {
			toID, err = resolveOrCreateNode(tx, rootID, e.ToSymbol)
			if err != nil {
				return fmt.Errorf("resolve target %s: %w", e.ToSymbol, err)
			}
		}
		if err := insertEdge(tx, fromID, toID, e.Kind, revID); err != nil {
			return fmt.Errorf("insert edge %sâ†’%s: %w", e.FromSymbol, e.ToSymbol, err)
		}
	}

	return tx.Commit()
}

// InvalidateFile invalida todas las aristas activas del nodo file correspondiente a relPath
// en la raÃ­z rootID. Se usa cuando un archivo fue eliminado en una raÃ­z vcs=none.
func (b *Builder) InvalidateFile(rootID, revID int64, relPath string) error {
	tx, err := b.store.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var fileNodeID int64
	err = tx.QueryRow(
		`SELECT id FROM nodes WHERE root_id=? AND file_path=? AND kind='file' LIMIT 1`,
		rootID, relPath,
	).Scan(&fileNodeID)
	if err == sql.ErrNoRows {
		return nil // file node no existe â†’ nada que invalidar
	}
	if err != nil {
		return fmt.Errorf("find file node %s: %w", relPath, err)
	}

	activeEdges, err := loadActiveEdgesFrom(tx, fileNodeID)
	if err != nil {
		return fmt.Errorf("load active edges for %s: %w", relPath, err)
	}
	for _, ae := range activeEdges {
		if err := invalidateEdge(tx, ae.id, revID); err != nil {
			return fmt.Errorf("invalidate edge %d: %w", ae.id, err)
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
// Always SELECT after UPSERT â€” LastInsertId is unreliable for ON CONFLICT DO UPDATE.
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
	id         int64
	fromSymbol string
	toSymbol   string
	kind       string
}

// loadActiveEdgesFrom returns active edges whose from_id = nodeID.
func loadActiveEdgesFrom(tx *sql.Tx, nodeID int64) ([]activeEdge, error) {
	rows, err := tx.Query(`
		SELECT e.id, nf.symbol, nt.symbol, e.kind
		FROM edges e
		JOIN nodes nf ON nf.id = e.from_id
		JOIN nodes nt ON nt.id = e.to_id
		WHERE e.from_id = ? AND e.valid_until_rev IS NULL
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []activeEdge
	for rows.Next() {
		var ae activeEdge
		if err := rows.Scan(&ae.id, &ae.fromSymbol, &ae.toSymbol, &ae.kind); err != nil {
			return nil, err
		}
		out = append(out, ae)
	}
	return out, rows.Err()
}

// loadActiveEdgesForFile returns active edges originating from any node that
// belongs to filePath (the file node itself plus any symbol defined in it).
// This is what UpsertFile uses so bi-temporal invalidation covers symbol->symbol
// edges (e.g. CloudFormation resource->resource), not just file->symbol edges.
func loadActiveEdgesForFile(tx *sql.Tx, rootID int64, filePath string) ([]activeEdge, error) {
	rows, err := tx.Query(`
		SELECT e.id, nf.symbol, nt.symbol, e.kind
		FROM edges e
		JOIN nodes nf ON nf.id = e.from_id
		JOIN nodes nt ON nt.id = e.to_id
		WHERE nf.root_id = ? AND nf.file_path = ? AND e.valid_until_rev IS NULL
	`, rootID, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []activeEdge
	for rows.Next() {
		var ae activeEdge
		if err := rows.Scan(&ae.id, &ae.fromSymbol, &ae.toSymbol, &ae.kind); err != nil {
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
//
// For TypeScript relative imports the extractor emits a resolved path without
// extension (e.g. "src/components/utils"). A second pass tries to match a file
// node whose file_path starts with that prefix, covering the common patterns
// "utils.ts", "utils/index.ts", "utils.js", etc.
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
	// Path-prefix fallback: symbol looks like a relative path (contains "/").
	// Match file nodes whose file_path equals symbol with a common extension appended,
	// or is an index file inside that directory.
	if strings.Contains(symbol, "/") {
		err2 := tx.QueryRow(
			`SELECT id FROM nodes WHERE root_id=? AND kind='file'
			 AND (file_path=? OR file_path LIKE ? OR file_path LIKE ?) LIMIT 1`,
			rootID, symbol,
			symbol+".%",           // src/utils.ts, src/utils.js, src/utils.tsx …
			symbol+"/index.%",     // src/utils/index.ts …
		).Scan(&id)
		if err2 == nil {
			return id, nil
		}
		if err2 != sql.ErrNoRows {
			return 0, err2
		}
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
