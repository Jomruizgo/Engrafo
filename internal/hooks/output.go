package hooks

import "encoding/json"

// hookOutput is written to stdout for Claude Code to parse.
type hookOutput struct {
	SystemMessage string `json:"systemMessage,omitempty"`
}

// BuildOutput formats the hook stdout JSON response.
// Empty message → no systemMessage key (no injection).
func BuildOutput(msg string) string {
	out := hookOutput{SystemMessage: msg}
	b, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return string(b)
}
