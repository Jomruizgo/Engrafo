// Package extractors contains language-specific AST extractors.
// Each extractor implements the Extractor interface defined in contract_test.go.
// Full implementation: feature/parser.
package extractors

// GoExtractor extracts nodes and edges from Go source files.
type GoExtractor struct{}
