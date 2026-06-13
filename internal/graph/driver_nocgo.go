//go:build !cgo

package graph

// BLOQUEANTE: gcc ausente — usando modernc.org/sqlite (puro Go) hasta que gcc esté disponible.
// Para swap a go-sqlite3: instalar gcc, eliminar este archivo y usar driver_cgo.go.
import _ "modernc.org/sqlite"

const sqliteDriver = "sqlite"
