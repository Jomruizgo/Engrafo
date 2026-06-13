package engrafo_test

// Smoke test: verifies that all main packages compile.
// This is the only test in feature/scaffolding.
// Subsequent steps add package-level tests.

import (
	"testing"

	_ "github.com/Jomruizgo/Engrafo/internal/engram"
	_ "github.com/Jomruizgo/Engrafo/internal/graph"
	_ "github.com/Jomruizgo/Engrafo/internal/hooks"
	_ "github.com/Jomruizgo/Engrafo/internal/mcp"
	_ "github.com/Jomruizgo/Engrafo/internal/parser"
	_ "github.com/Jomruizgo/Engrafo/internal/parser/extractors"
	_ "github.com/Jomruizgo/Engrafo/internal/watcher"
)

func TestSmoke(t *testing.T) {
	// Arrange: all packages imported above
	// Act:     the import itself is the action (if it compiles, packages are valid)
	// Assert:  compilation success = test passes
	t.Log("all packages compile")
}
