//go:build cgo

package extractors_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jomruizgo/Engrafo/v2/internal/parser"
	"github.com/Jomruizgo/Engrafo/v2/internal/parser/extractors"
)

// nodePresent returns true if result contains a node with the given symbol and kind.
func nodePresent(result *parser.Result, symbol, kind string) bool {
	for _, n := range result.Nodes {
		if n.Symbol == symbol && n.Kind == kind {
			return true
		}
	}
	return false
}

// edgePresent returns true if result contains an edge with the given toSymbol and kind.
func edgePresent(result *parser.Result, toSymbol, kind string) bool {
	for _, e := range result.Edges {
		if e.ToSymbol == toSymbol && e.Kind == kind {
			return true
		}
	}
	return false
}

// fixture returns the absolute path to testdata/fixtures/<lang>/<file>.
func fixture(lang, file string) string {
	abs, _ := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "fixtures", lang, file))
	return abs
}

// ----- Go extractor tests -----

func TestGoExtractor(t *testing.T) {
	path := fixture("go", "simple.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	type nodeCase struct{ symbol, kind string }
	type edgeCase struct{ toSymbol, kind string }

	tests := []struct {
		name      string
		wantNodes []nodeCase
		wantEdges []edgeCase
	}{
		{
			name: "simple.go symbols and imports",
			wantNodes: []nodeCase{
				{"fixtures", "package"},
				{"UserService", "class"},
				{"UserRepository", "interface"},
				{"NewUserService", "function"},
				{"GetName", "method"},
				{"helperFunc", "function"},
			},
			wantEdges: []edgeCase{
				{"fmt", "imports"},
				{"errors", "imports"},
			},
		},
	}

	ext := &extractors.GoExtractor{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: fixture file read above
			// Act
			result, err := ext.Extract(path, src)
			// Assert
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			for _, want := range tc.wantNodes {
				if !nodePresent(result, want.symbol, want.kind) {
					t.Errorf("missing node: symbol=%q kind=%q", want.symbol, want.kind)
					t.Logf("got nodes: %v", result.Nodes)
				}
			}
			for _, want := range tc.wantEdges {
				if !edgePresent(result, want.toSymbol, want.kind) {
					t.Errorf("missing edge: to=%q kind=%q", want.toSymbol, want.kind)
					t.Logf("got edges: %v", result.Edges)
				}
			}
		})
	}
}

// ----- TypeScript extractor tests -----

func TestTypeScriptExtractor(t *testing.T) {
	path := fixture("typescript", "simple.ts")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	type nodeCase struct{ symbol, kind string }
	type edgeCase struct{ toSymbol, kind string }

	tests := []struct {
		name      string
		wantNodes []nodeCase
		wantEdges []edgeCase
	}{
		{
			name: "simple.ts symbols and imports",
			wantNodes: []nodeCase{
				{"Repository", "interface"},
				{"UserService", "class"},
				{"getName", "method"},
				{"createService", "function"},
			},
			wantEdges: []edgeCase{
				{"events", "imports"},
				{"EventEmitter", "inherits"},
			},
		},
	}

	ext := &extractors.TypeScriptExtractor{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: fixture file read above
			// Act
			result, err := ext.Extract(path, src)
			// Assert
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			for _, want := range tc.wantNodes {
				if !nodePresent(result, want.symbol, want.kind) {
					t.Errorf("missing node: symbol=%q kind=%q", want.symbol, want.kind)
					t.Logf("got nodes: %v", result.Nodes)
				}
			}
			for _, want := range tc.wantEdges {
				if !edgePresent(result, want.toSymbol, want.kind) {
					t.Errorf("missing edge: to=%q kind=%q", want.toSymbol, want.kind)
					t.Logf("got edges: %v", result.Edges)
				}
			}
		})
	}
}

// ----- Python extractor tests -----

func TestPythonExtractor(t *testing.T) {
	path := fixture("python", "simple.py")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	type nodeCase struct{ symbol, kind string }
	type edgeCase struct{ toSymbol, kind string }

	tests := []struct {
		name      string
		wantNodes []nodeCase
		wantEdges []edgeCase
	}{
		{
			name: "simple.py symbols and imports",
			wantNodes: []nodeCase{
				{"UserService", "class"},
				{"__init__", "method"},
				{"get_name", "method"},
				{"create_service", "function"},
			},
			wantEdges: []edgeCase{
				{"os", "imports"},
				{"typing", "imports"},
			},
		},
	}

	ext := &extractors.PythonExtractor{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: fixture file read above
			// Act
			result, err := ext.Extract(path, src)
			// Assert
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			for _, want := range tc.wantNodes {
				if !nodePresent(result, want.symbol, want.kind) {
					t.Errorf("missing node: symbol=%q kind=%q", want.symbol, want.kind)
					t.Logf("got nodes: %v", result.Nodes)
				}
			}
			for _, want := range tc.wantEdges {
				if !edgePresent(result, want.toSymbol, want.kind) {
					t.Errorf("missing edge: to=%q kind=%q", want.toSymbol, want.kind)
					t.Logf("got edges: %v", result.Edges)
				}
			}
		})
	}
}
