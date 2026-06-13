package parser

import "path/filepath"

// Language identifies a supported source language.
type Language string

const (
	LangGo         Language = "go"
	LangTypeScript Language = "typescript"
	LangPython     Language = "python"
)

// Detect returns the Language for the given file path based on extension.
// Returns "" when the extension is unknown.
func Detect(filename string) Language {
	switch filepath.Ext(filename) {
	case ".go":
		return LangGo
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		return LangTypeScript
	case ".py", ".pyw":
		return LangPython
	default:
		return ""
	}
}
