//go:build cgo

package extractors

import (
	"fmt"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"github.com/Jomruizgo/Engrafo/internal/parser"
)

// TypeScriptExtractor extracts nodes and edges from TypeScript/JavaScript source files.
type TypeScriptExtractor struct {
	lang *tree_sitter.Language
}

func (e *TypeScriptExtractor) init() *tree_sitter.Language {
	if e.lang == nil {
		e.lang = tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	}
	return e.lang
}

func (e *TypeScriptExtractor) Language() parser.Language { return parser.LangTypeScript }

func (e *TypeScriptExtractor) Extract(filePath string, source []byte) (*parser.Result, error) {
	lang := e.init()
	p, err := newParser(lang)
	if err != nil {
		return nil, fmt.Errorf("typescript extractor init: %w", err)
	}
	defer p.Close()

	tree := p.Parse(source, nil)
	defer tree.Close()
	root := tree.RootNode()

	var nodes []parser.Node
	var edges []parser.Edge

	// class declarations
	for _, cap := range queryAll(lang, root, source,
		`(class_declaration name: (type_identifier) @name)`, "name") {
		nodes = append(nodes, parser.Node{
			Symbol:    cap.text,
			Kind:      "class",
			FilePath:  filePath,
			LineStart: int(cap.startLine + 1),
			LineEnd:   int(cap.endLine + 1),
			Language:  string(parser.LangTypeScript),
		})
	}

	// interface declarations
	for _, cap := range queryAll(lang, root, source,
		`(interface_declaration name: (type_identifier) @name)`, "name") {
		nodes = append(nodes, parser.Node{
			Symbol:    cap.text,
			Kind:      "interface",
			FilePath:  filePath,
			LineStart: int(cap.startLine + 1),
			LineEnd:   int(cap.endLine + 1),
			Language:  string(parser.LangTypeScript),
		})
	}

	// top-level function declarations
	for _, cap := range queryAll(lang, root, source,
		`(function_declaration name: (identifier) @name)`, "name") {
		nodes = append(nodes, parser.Node{
			Symbol:    cap.text,
			Kind:      "function",
			FilePath:  filePath,
			LineStart: int(cap.startLine + 1),
			LineEnd:   int(cap.endLine + 1),
			Language:  string(parser.LangTypeScript),
		})
	}

	// method definitions (inside class body)
	for _, cap := range queryAll(lang, root, source,
		`(method_definition name: (property_identifier) @name)`, "name") {
		// skip constructor — it's a language keyword, not a user-defined method symbol
		if cap.text == "constructor" {
			continue
		}
		nodes = append(nodes, parser.Node{
			Symbol:    cap.text,
			Kind:      "method",
			FilePath:  filePath,
			LineStart: int(cap.startLine + 1),
			LineEnd:   int(cap.endLine + 1),
			Language:  string(parser.LangTypeScript),
		})
	}

	// import edges — source is a string literal like "'events'"
	for _, cap := range queryAll(lang, root, source,
		`(import_statement source: (string) @src)`, "src") {
		raw := strings.Trim(cap.text, `"'`)
		// use last segment of the module path
		parts := strings.Split(raw, "/")
		sym := parts[len(parts)-1]
		edges = append(edges, parser.Edge{
			FromSymbol: filePath,
			ToSymbol:   sym,
			Kind:       "imports",
		})
	}

	// inheritance edges — class Foo extends Bar → inherits
	for _, cap := range queryAll(lang, root, source,
		`(extends_clause value: (identifier) @parent)`, "parent") {
		edges = append(edges, parser.Edge{
			FromSymbol: filePath,
			ToSymbol:   cap.text,
			Kind:       "inherits",
		})
	}

	return &parser.Result{Nodes: nodes, Edges: edges}, nil
}
