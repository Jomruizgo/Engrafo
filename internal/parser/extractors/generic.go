package extractors

import "github.com/Jomruizgo/Engrafo/internal/parser"

// GenericExtractor is a fallback for languages without a dedicated extractor.
// It produces a single file-level node with kind "file" and no edges.
type GenericExtractor struct {
	lang parser.Language
}

func NewGenericExtractor(lang parser.Language) *GenericExtractor {
	return &GenericExtractor{lang: lang}
}

func (e *GenericExtractor) Language() parser.Language { return e.lang }

func (e *GenericExtractor) Extract(filePath string, _ []byte) (*parser.Result, error) {
	return &parser.Result{
		Nodes: []parser.Node{{
			Symbol:   filePath,
			Kind:     "file",
			FilePath: filePath,
			Language: string(e.lang),
		}},
	}, nil
}
