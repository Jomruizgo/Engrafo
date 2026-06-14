//go:build cgo

package extractors_test

// TestExtractorContract verifies at compile time that all three extractors
// implement the parser.Extractor interface.
// If this file compiles, the contract is satisfied.

import (
	"testing"

	"github.com/Jomruizgo/Engrafo/v2/internal/parser"
	"github.com/Jomruizgo/Engrafo/v2/internal/parser/extractors"
)

func TestExtractorContract(t *testing.T) {
	// Arrange: compile-time interface assertions
	var _ parser.Extractor = (*extractors.GoExtractor)(nil)
	var _ parser.Extractor = (*extractors.TypeScriptExtractor)(nil)
	var _ parser.Extractor = (*extractors.PythonExtractor)(nil)

	// Act + Assert: instantiate each extractor and verify Language()
	cases := []struct {
		name     string
		ext      parser.Extractor
		wantLang parser.Language
	}{
		{"Go", &extractors.GoExtractor{}, parser.LangGo},
		{"TypeScript", &extractors.TypeScriptExtractor{}, parser.LangTypeScript},
		{"Python", &extractors.PythonExtractor{}, parser.LangPython},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange (extractor instantiated above)
			// Act
			got := tc.ext.Language()
			// Assert
			if got != tc.wantLang {
				t.Errorf("Language(): want %q, got %q", tc.wantLang, got)
			}
		})
	}
}
