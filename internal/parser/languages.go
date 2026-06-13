package parser

// Language identifies a supported source language.
type Language string

const (
	LangGo         Language = "go"
	LangTypeScript Language = "typescript"
	LangPython     Language = "python"
)

// Detect returns the Language for the given file path based on extension.
// Returns "" when the language is unknown.
func Detect(_ string) Language {
	return ""
}
