//go:build cgo

package graph

// Usar modernc.org/sqlite también en modo CGO para garantizar FTS5 disponible.
// mattn/go-sqlite3 requiere -DSQLITE_ENABLE_FTS5 explícito; modernc lo incluye por defecto.
import _ "modernc.org/sqlite"

const sqliteDriver = "sqlite"
