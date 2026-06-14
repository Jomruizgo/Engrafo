//go:build cgo

package extractors

import (
	"fmt"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_golang "github.com/tree-sitter/tree-sitter-go/bindings/go"

	"github.com/Jomruizgo/Engrafo/v2/internal/parser"
)

// GoExtractor extracts nodes and edges from Go source files using tree-sitter.
type GoExtractor struct {
	lang *tree_sitter.Language
}

func (e *GoExtractor) init() *tree_sitter.Language {
	if e.lang == nil {
		e.lang = tree_sitter.NewLanguage(tree_sitter_golang.Language())
	}
	return e.lang
}

func (e *GoExtractor) Language() parser.Language { return parser.LangGo }

func (e *GoExtractor) Extract(filePath string, source []byte) (*parser.Result, error) {
	lang := e.init()
	p, err := newParser(lang)
	if err != nil {
		return nil, fmt.Errorf("go extractor init: %w", err)
	}
	defer p.Close()

	tree := p.Parse(source, nil)
	defer tree.Close()
	root := tree.RootNode()

	var nodes []parser.Node
	var edges []parser.Edge

	// package declaration
	pkgName := queryFirst(lang, root, source,
		`(package_clause (package_identifier) @name)`, "name")
	if pkgName != "" {
		nodes = append(nodes, parser.Node{
			Symbol:   pkgName,
			Kind:     "package",
			FilePath: filePath,
			Language: string(parser.LangGo),
		})
	}

	// top-level functions
	for _, cap := range queryAll(lang, root, source,
		`(function_declaration name: (identifier) @name)`, "name") {
		nodes = append(nodes, parser.Node{
			Symbol:    cap.text,
			Kind:      "function",
			FilePath:  filePath,
			LineStart: int(cap.startLine + 1),
			LineEnd:   int(cap.endLine + 1),
			Language:  string(parser.LangGo),
		})
	}

	// methods
	for _, cap := range queryAll(lang, root, source,
		`(method_declaration name: (field_identifier) @name)`, "name") {
		nodes = append(nodes, parser.Node{
			Symbol:    cap.text,
			Kind:      "method",
			FilePath:  filePath,
			LineStart: int(cap.startLine + 1),
			LineEnd:   int(cap.endLine + 1),
			Language:  string(parser.LangGo),
		})
	}

	// struct types â†’ kind "class"
	for _, cap := range queryAll(lang, root, source,
		`(type_declaration (type_spec name: (type_identifier) @name type: (struct_type)))`, "name") {
		nodes = append(nodes, parser.Node{
			Symbol:    cap.text,
			Kind:      "class",
			FilePath:  filePath,
			LineStart: int(cap.startLine + 1),
			LineEnd:   int(cap.endLine + 1),
			Language:  string(parser.LangGo),
		})
	}

	// interface types
	for _, cap := range queryAll(lang, root, source,
		`(type_declaration (type_spec name: (type_identifier) @name type: (interface_type)))`, "name") {
		nodes = append(nodes, parser.Node{
			Symbol:    cap.text,
			Kind:      "interface",
			FilePath:  filePath,
			LineStart: int(cap.startLine + 1),
			LineEnd:   int(cap.endLine + 1),
			Language:  string(parser.LangGo),
		})
	}

	// import edges
	for _, cap := range queryAll(lang, root, source,
		`(import_spec path: (interpreted_string_literal) @path)`, "path") {
		raw := strings.Trim(cap.text, `"`)
		// use last path segment as the import symbol (e.g. "github.com/foo/bar" â†’ "bar")
		parts := strings.Split(raw, "/")
		sym := parts[len(parts)-1]
		edges = append(edges, parser.Edge{
			FromSymbol: filePath,
			ToSymbol:   sym,
			Kind:       "imports",
		})
	}

	return &parser.Result{Nodes: nodes, Edges: edges}, nil
}
