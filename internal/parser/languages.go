package parser

import "path/filepath"

// Language identifies a supported source language.
type Language string

const (
	LangGo             Language = "go"
	LangTypeScript     Language = "typescript"
	LangPython         Language = "python"
	LangCloudFormation Language = "cloudformation"
)

// Detect returns the Language for the given file path based on extension.
// Returns "" when the extension is unknown.
func Detect(filename string) Language {
	switch filepath.Ext(filename) {
	case ".go":
		return LangGo
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue":
		return LangTypeScript
	case ".py", ".pyw":
		return LangPython
	case ".yaml", ".yml":
		// Only CloudFormation/SAM templates produce nodes; the extractor returns
		// an empty result for non-template YAML (CI configs, k8s, etc.).
		return LangCloudFormation
	default:
		return ""
	}
}
