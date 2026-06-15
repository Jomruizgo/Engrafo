package extractors

import "strings"

// extractVueScript returns only the contents of <script> blocks from a Vue SFC,
// blanking template/style/tag lines so tree-sitter line numbers still map to the
// original .vue file. Handles multiple blocks (e.g. <script> + <script setup>).
//
// Build-tag free (pure string manipulation, no tree-sitter) so it is testable
// without CGO.
func extractVueScript(source []byte) []byte {
	lines := strings.Split(string(source), "\n")
	inScript := false
	for i, ln := range lines {
		lower := strings.ToLower(strings.TrimSpace(ln))
		switch {
		case !inScript && strings.HasPrefix(lower, "<script"):
			inScript = true
			lines[i] = "" // blank the opening tag line
		case inScript && strings.HasPrefix(lower, "</script>"):
			inScript = false
			lines[i] = "" // blank the closing tag line
		case !inScript:
			lines[i] = "" // template / style / anything outside <script>
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
