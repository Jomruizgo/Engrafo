-- schema version for migrations
CREATE TABLE IF NOT EXISTS schema_version (
    version     INTEGER PRIMARY KEY,
    applied_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- nodes: any identifiable symbol
CREATE TABLE IF NOT EXISTS nodes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol      TEXT NOT NULL,
    kind        TEXT NOT NULL,      -- "function"|"method"|"class"|"interface"|"file"|"package"
    file_path   TEXT NOT NULL,      -- relative to repo root
    line_start  INTEGER,
    line_end    INTEGER,
    signature   TEXT,
    language    TEXT NOT NULL,      -- "go"|"typescript"|"python"
    checksum    TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_symbol_file
    ON nodes(symbol, file_path, kind);

CREATE INDEX IF NOT EXISTS idx_nodes_file   ON nodes(file_path);
CREATE INDEX IF NOT EXISTS idx_nodes_symbol ON nodes(symbol);

-- edges: bi-temporal — never deleted, only invalidated
CREATE TABLE IF NOT EXISTS edges (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    from_id             INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    to_id               INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL,          -- "calls"|"imports"|"inherits"|"implements"|"uses"
    valid_from_commit   TEXT NOT NULL,
    valid_until_commit  TEXT,                   -- NULL = active
    invalidated_reason  TEXT,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- uniqueness only among ACTIVE edges
CREATE UNIQUE INDEX IF NOT EXISTS idx_edges_unique_active
    ON edges(from_id, to_id, kind) WHERE valid_until_commit IS NULL;

CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_id);
CREATE INDEX IF NOT EXISTS idx_edges_to   ON edges(to_id);

-- FTS for symbol search
CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
    symbol,
    file_path,
    signature,
    content=nodes,
    content_rowid=id
);

-- Triggers to keep nodes_fts in sync with nodes table
CREATE TRIGGER IF NOT EXISTS nodes_fts_ai AFTER INSERT ON nodes BEGIN
    INSERT INTO nodes_fts(rowid, symbol, file_path, signature)
    VALUES (new.id, new.symbol, new.file_path, new.signature);
END;

CREATE TRIGGER IF NOT EXISTS nodes_fts_ad AFTER DELETE ON nodes BEGIN
    INSERT INTO nodes_fts(nodes_fts, rowid, symbol, file_path, signature)
    VALUES ('delete', old.id, old.symbol, old.file_path, old.signature);
END;

CREATE TRIGGER IF NOT EXISTS nodes_fts_au AFTER UPDATE ON nodes BEGIN
    INSERT INTO nodes_fts(nodes_fts, rowid, symbol, file_path, signature)
    VALUES ('delete', old.id, old.symbol, old.file_path, old.signature);
    INSERT INTO nodes_fts(rowid, symbol, file_path, signature)
    VALUES (new.id, new.symbol, new.file_path, new.signature);
END;

-- engram anchor table
CREATE TABLE IF NOT EXISTS engram_anchors (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id         INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    engram_obs_id   TEXT NOT NULL,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_anchors_node ON engram_anchors(node_id);
CREATE INDEX IF NOT EXISTS idx_anchors_obs  ON engram_anchors(engram_obs_id);

-- index metadata
CREATE TABLE IF NOT EXISTS index_meta (
    key     TEXT PRIMARY KEY,
    value   TEXT
);
-- keys: "last_commit_hash", "repo_root", "indexed_at"
