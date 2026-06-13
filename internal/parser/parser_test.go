//go:build cgo

package parser_test

import (
	"os"
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/parser"
	"github.com/Jomruizgo/Engrafo/internal/parser/extractors"
)

func newTestParser() *parser.Parser {
	return parser.New(
		&extractors.GoExtractor{},
		&extractors.TypeScriptExtractor{},
		&extractors.PythonExtractor{},
	)
}

func TestParseContentMatchesParseFile(t *testing.T) {
	// Arrange: read the Go fixture and parse both ways
	fixture := "../../testdata/fixtures/go/simple.go"
	content, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	p := newTestParser()

	// Act: ParseFile
	fromFile, err := p.ParseFile(fixture)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Act: ParseContent
	fromContent, err := p.ParseContent(fixture, content)
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}

	// Assert: same number of nodes and edges
	if len(fromFile.Nodes) != len(fromContent.Nodes) {
		t.Errorf("node count: ParseFile=%d ParseContent=%d",
			len(fromFile.Nodes), len(fromContent.Nodes))
	}
	if len(fromFile.Edges) != len(fromContent.Edges) {
		t.Errorf("edge count: ParseFile=%d ParseContent=%d",
			len(fromFile.Edges), len(fromContent.Edges))
	}
}

func TestParseContentUnsupportedLanguage(t *testing.T) {
	// Arrange
	p := newTestParser()

	// Act: pass a .rb file (not supported)
	_, err := p.ParseContent("file.rb", []byte("class Foo; end"))

	// Assert: returns error (no extractor)
	if err == nil {
		t.Error("want error for unsupported language, got nil")
	}
}
