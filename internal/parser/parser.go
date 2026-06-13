// Package parser coordinates multi-language parsing with go-tree-sitter.
// Full implementation: feature/parser.
package parser

// Node represents an extracted symbol from source code.
type Node struct {
	Symbol    string
	Kind      string // "function"|"method"|"class"|"interface"|"file"|"package"
	FilePath  string
	LineStart int
	LineEnd   int
	Signature string
	Language  string
	Checksum  string
}

// Edge represents a relationship between two nodes.
type Edge struct {
	FromSymbol string
	ToSymbol   string
	Kind       string // "calls"|"imports"|"inherits"|"implements"|"uses"
}

// Result is the output of parsing a single file.
type Result struct {
	Nodes []Node
	Edges []Edge
}

// Parser coordinates extraction across languages.
type Parser struct{}

// ParseFile parses a single file and returns its nodes and edges.
func (p *Parser) ParseFile(_ string) (*Result, error) {
	return &Result{}, nil
}
