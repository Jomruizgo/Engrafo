// Package engram bridges engrafo with engram memory observations.
package engram

import (
	"regexp"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
)

// Bridge links engram observations to graph nodes.
type Bridge struct {
	store *graph.Store
}

// New creates a Bridge backed by the given Store.
func New(s *graph.Store) *Bridge {
	return &Bridge{store: s}
}

// symbolToken matches identifier-like and path-like tokens in free text:
// camelCase/PascalCase identifiers, snake_case, dotted file names, and slash paths.
var symbolToken = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_./-]*`)

// AutoAnchor extracts symbol candidates from an observation's text and anchors
// the observation to every candidate that exists as a node in the graph.
// rootName filters to a single root; "" considers all roots.
//
// Matching is case-sensitive exact: prose words ("decidimos", "usar") are lowercase
// and won't match code symbols (AuthService, handler.ts), so noise is filtered
// structurally. AnchorObservations silently skips tokens with no matching node, so
// only real symbols ever produce an anchor. Returns the number of anchors created.
func (b *Bridge) AutoAnchor(obsID, text, rootName string) (int, error) {
	if obsID == "" || text == "" {
		return 0, nil
	}
	symbols := extractSymbolCandidates(text)
	if len(symbols) == 0 {
		return 0, nil
	}
	return b.store.AnchorObservations(obsID, symbols, rootName)
}

// extractSymbolCandidates returns deduped tokens from text that look like code
// symbols or file paths: must contain an uppercase letter (camelCase/PascalCase),
// an underscore (snake_case, e.g. Python), a "/", or a ".". This is a cheap
// pre-filter to bound DB lookups; correctness comes from the graph node match in
// AnchorObservations.
func extractSymbolCandidates(text string) []string {
	const maxCandidates = 64
	seen := make(map[string]bool)
	var out []string
	for _, tok := range symbolToken.FindAllString(text, -1) {
		if len(tok) < 4 {
			continue
		}
		if !looksLikeSymbol(tok) {
			continue
		}
		if seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
		if len(out) >= maxCandidates {
			break
		}
	}
	return out
}

func looksLikeSymbol(tok string) bool {
	for _, r := range tok {
		if r >= 'A' && r <= 'Z' {
			return true
		}
		if r == '_' || r == '/' || r == '.' {
			return true
		}
	}
	return false
}
