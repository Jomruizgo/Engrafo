// Package parser coordinates multi-language parsing with go-tree-sitter.
package parser

import (
	"fmt"
	"os"
	"path/filepath"
)

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

// Edge represents a directional relationship between two symbols.
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

// Extractor parses one source file and returns its nodes and edges.
// Each language has a dedicated implementation in the extractors/ package.
type Extractor interface {
	// Language returns the language this extractor handles.
	Language() Language
	// Extract parses the file at filePath (content: source) and returns nodes/edges.
	Extract(filePath string, source []byte) (*Result, error)
}

// Parser coordinates extraction across registered languages.
type Parser struct {
	extractors map[Language]Extractor
}

// New creates a Parser with the given extractors registered.
func New(exts ...Extractor) *Parser {
	m := make(map[Language]Extractor, len(exts))
	for _, e := range exts {
		m[e.Language()] = e
	}
	return &Parser{extractors: m}
}

// ParseFile detects the language of filePath, reads it, and extracts nodes/edges.
// Returns an error when the language is unsupported.
func (p *Parser) ParseFile(filePath string) (*Result, error) {
	lang := Detect(filePath)
	ext, ok := p.extractors[lang]
	if !ok {
		return nil, fmt.Errorf("no extractor for language %q (file: %s)", lang, filepath.Base(filePath))
	}
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}
	return ext.Extract(filePath, src)
}
