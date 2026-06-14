//go:build cgo

package extractors

import (
	"fmt"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"

	"github.com/Jomruizgo/Engrafo/v2/internal/parser"
)

// PythonExtractor extracts nodes and edges from Python source files using tree-sitter.
type PythonExtractor struct {
	lang *tree_sitter.Language
}

func (e *PythonExtractor) init() *tree_sitter.Language {
	if e.lang == nil {
		e.lang = tree_sitter.NewLanguage(tree_sitter_python.Language())
	}
	return e.lang
}

func (e *PythonExtractor) Language() parser.Language { return parser.LangPython }

func (e *PythonExtractor) Extract(filePath string, source []byte) (*parser.Result, error) {
	lang := e.init()
	p, err := newParser(lang)
	if err != nil {
		return nil, fmt.Errorf("python extractor init: %w", err)
	}
	defer p.Close()

	tree := p.Parse(source, nil)
	defer tree.Close()
	root := tree.RootNode()

	var nodes []parser.Node
	var edges []parser.Edge

	// class definitions
	for _, cap := range queryAll(lang, root, source,
		`(class_definition name: (identifier) @name)`, "name") {
		nodes = append(nodes, parser.Node{
			Symbol:    cap.text,
			Kind:      "class",
			FilePath:  filePath,
			LineStart: int(cap.startLine + 1),
			LineEnd:   int(cap.endLine + 1),
			Language:  string(parser.LangPython),
		})
	}

	// methods (function_definition inside a class body)
	for _, cap := range queryAll(lang, root, source,
		`(class_definition body: (block (function_definition name: (identifier) @name)))`, "name") {
		nodes = append(nodes, parser.Node{
			Symbol:    cap.text,
			Kind:      "method",
			FilePath:  filePath,
			LineStart: int(cap.startLine + 1),
			LineEnd:   int(cap.endLine + 1),
			Language:  string(parser.LangPython),
		})
	}

	// top-level functions (direct children of module)
	for _, cap := range queryAll(lang, root, source,
		`(module (function_definition name: (identifier) @name))`, "name") {
		nodes = append(nodes, parser.Node{
			Symbol:    cap.text,
			Kind:      "function",
			FilePath:  filePath,
			LineStart: int(cap.startLine + 1),
			LineEnd:   int(cap.endLine + 1),
			Language:  string(parser.LangPython),
		})
	}

	// file-level import edges: "import os" -> file -[imports]-> os
	for _, cap := range queryAll(lang, root, source,
		`(import_statement name: (dotted_name) @mod)`, "mod") {
		parts := strings.Split(cap.text, ".")
		edges = append(edges, parser.Edge{
			FromSymbol: filePath,
			ToSymbol:   parts[0],
			Kind:       "imports",
		})
	}

	// file-level import edges: "from X import ..." -> file -[imports]-> X
	for _, cap := range queryAll(lang, root, source,
		`(import_from_statement module_name: (dotted_name) @mod)`, "mod") {
		parts := strings.Split(cap.text, ".")
		edges = append(edges, parser.Edge{
			FromSymbol: filePath,
			ToSymbol:   parts[0],
			Kind:       "imports",
		})
	}

	// symbol-level uses edges: "from X import AuthService, UserError" emits:
	//   filePath -[uses]-> AuthService
	//   filePath -[uses]-> UserError
	// Handles both absolute ("from services import X") and relative ("from . import X").
	// The builder resolves each name against existing non-external nodes in the
	// same root; matches give accurate used_by data for cg_impact.
	for _, cap := range queryAll(lang, root, source,
		`(import_from_statement name: (dotted_name) @name)`, "name") {
		edges = append(edges, parser.Edge{
			FromSymbol: filePath,
			ToSymbol:   cap.text,
			Kind:       "uses",
		})
	}

	return &parser.Result{Nodes: nodes, Edges: edges}, nil
}
