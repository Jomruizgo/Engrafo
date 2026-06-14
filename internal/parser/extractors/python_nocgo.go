//go:build !cgo

package extractors

import (
	"errors"

	"github.com/Jomruizgo/Engrafo/v2/internal/parser"
)

// PythonExtractor stub for non-CGO builds.
// BLOQUEANTE: tree-sitter requiere CGO_ENABLED=1. Compile con gcc en PATH.
type PythonExtractor struct{}

func (e *PythonExtractor) Language() parser.Language { return parser.LangPython }

func (e *PythonExtractor) Extract(_ string, _ []byte) (*parser.Result, error) {
	return nil, errors.New("python extractor requires CGO_ENABLED=1 (tree-sitter needs gcc)")
}
