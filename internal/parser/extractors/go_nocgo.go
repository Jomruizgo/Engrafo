//go:build !cgo

package extractors

import (
	"errors"

	"github.com/Jomruizgo/Engrafo/internal/parser"
)

// GoExtractor stub for non-CGO builds.
// BLOQUEANTE: tree-sitter requiere CGO_ENABLED=1. Compile con gcc en PATH.
type GoExtractor struct{}

func (e *GoExtractor) Language() parser.Language { return parser.LangGo }

func (e *GoExtractor) Extract(_ string, _ []byte) (*parser.Result, error) {
	return nil, errors.New("go extractor requires CGO_ENABLED=1 (tree-sitter needs gcc)")
}
