-- Schema v2: multi-repo + bi-temporal revisions
-- Para bases nuevas. Migraciones desde v1 las hace store.migrateV1ToV2().

CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Raíces indexadas (una por repo en modo multi-repo; exactamente una en modo single-repo)
CREATE TABLE IF NOT EXISTS roots (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    name             TEXT NOT NULL UNIQUE,
    rel_path         TEXT NOT NULL,
    abs_root         TEXT NOT NULL,
    remote_url       TEXT,
    default_branch   TEXT,
    vcs              TEXT NOT NULL DEFAULT 'git',
    last_commit_hash TEXT,
    indexed_at       DATETIME
);

-- Revisiones: coordenada temporal del modelo bi-temporal
CREATE TABLE IF NOT EXISTS revisions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    root_id     INTEGER NOT NULL REFERENCES roots(id) ON DELETE CASCADE,
    source      TEXT NOT NULL,       -- 'git' | 'checksum' | 'init'
    commit_hash TEXT,                -- NULL salvo source='git'
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_revisions_root ON revisions(root_id);

-- Nodos: símbolos identificables (con scope de raíz)
CREATE TABLE IF NOT EXISTS nodes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    root_id     INTEGER NOT NULL REFERENCES roots(id) ON DELETE CASCADE,
    symbol      TEXT NOT NULL,
    kind        TEXT NOT NULL,
    file_path   TEXT NOT NULL,
    line_start  INTEGER,
    line_end    INTEGER,
    signature   TEXT,
    language    TEXT NOT NULL,
    checksum    TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_root_symbol_file
    ON nodes(root_id, symbol, file_path, kind);
CREATE INDEX IF NOT EXISTS idx_nodes_file   ON nodes(file_path);
CREATE INDEX IF NOT EXISTS idx_nodes_symbol ON nodes(symbol);
CREATE INDEX IF NOT EXISTS idx_nodes_root   ON nodes(root_id);

-- Aristas: bi-temporal — nunca se borran, solo se invalidan
CREATE TABLE IF NOT EXISTS edges (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    from_id            INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    to_id              INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    kind               TEXT NOT NULL,
    valid_from_rev     INTEGER NOT NULL REFERENCES revisions(id),
    valid_until_rev    INTEGER REFERENCES revisions(id),   -- NULL = activa
    invalidated_reason TEXT,
    created_at         DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_edges_unique_active
    ON edges(from_id, to_id, kind) WHERE valid_until_rev IS NULL;
CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_id);
CREATE INDEX IF NOT EXISTS idx_edges_to   ON edges(to_id);

-- FTS para búsqueda de símbolos
CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
    symbol,
    file_path,
    signature,
    content=nodes,
    content_rowid=id
);

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

-- Anclas de observaciones engram
CREATE TABLE IF NOT EXISTS engram_anchors (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id       INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    engram_obs_id TEXT NOT NULL,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_anchors_node ON engram_anchors(node_id);
CREATE INDEX IF NOT EXISTS idx_anchors_obs  ON engram_anchors(engram_obs_id);

-- Metadatos de índice (legacy; repo_root/last_commit_hash migrados a roots en v2)
CREATE TABLE IF NOT EXISTS index_meta (
    key   TEXT PRIMARY KEY,
    value TEXT
);
