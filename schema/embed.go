// Package schema exposes the SQLite schema SQL for embedding.
package schema

import _ "embed"

//go:embed schema.sql
var SQL string
