//go:build cgo

package extractors

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// capture holds the text and line range of a tree-sitter match.
type capture struct {
	text      string
	startLine uint // 0-based
	endLine   uint // 0-based
}

// queryAll runs queryStr against node and returns all captures named captureName.
func queryAll(lang *tree_sitter.Language, node *tree_sitter.Node, source []byte, queryStr, captureName string) []capture {
	q, qErr := tree_sitter.NewQuery(lang, queryStr)
	if qErr != nil {
		return nil
	}
	defer q.Close()

	// find the capture index for captureName
	names := q.CaptureNames()
	captureIdx := -1
	for i, n := range names {
		if n == captureName {
			captureIdx = i
			break
		}
	}
	if captureIdx < 0 {
		return nil
	}

	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()
	matches := cursor.Matches(q, node, source)

	var results []capture
	for match := matches.Next(); match != nil; match = matches.Next() {
		for _, cap := range match.Captures {
			if int(cap.Index) == captureIdx {
				sp := cap.Node.StartPosition()
				ep := cap.Node.EndPosition()
				results = append(results, capture{
					text:      cap.Node.Utf8Text(source),
					startLine: sp.Row,
					endLine:   ep.Row,
				})
			}
		}
	}
	return results
}

// queryFirst returns the text of the first capture named captureName, or "".
func queryFirst(lang *tree_sitter.Language, node *tree_sitter.Node, source []byte, queryStr, captureName string) string {
	caps := queryAll(lang, node, source, queryStr, captureName)
	if len(caps) > 0 {
		return caps[0].text
	}
	return ""
}

// newLanguage wraps the unsafe.Pointer returned by grammar bindings.
func newParser(lang *tree_sitter.Language) (*tree_sitter.Parser, error) {
	p := tree_sitter.NewParser()
	return p, p.SetLanguage(lang)
}
