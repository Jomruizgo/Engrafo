//go:build !cgo

package extractors

import (
	"errors"

	"github.com/Jomruizgo/Engrafo/v2/internal/parser"
)

// TypeScriptExtractor stub for non-CGO builds.
// BLOQUEANTE: tree-sitter requiere CGO_ENABLED=1. Compile con gcc en PATH.
type TypeScriptExtractor struct{}

func (e *TypeScriptExtractor) Language() parser.Language { return parser.LangTypeScript }

func (e *TypeScriptExtractor) Extract(_ string, _ []byte) (*parser.Result, error) {
	return nil, errors.New("typescript extractor requires CGO_ENABLED=1 (tree-sitter needs gcc)")
}
