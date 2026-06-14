package schema_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Jomruizgo/Engrafo/v2/internal/version"
)

// requiredPluginFields are the top-level keys that plugin.json must contain.
var requiredPluginFields = []string{
	"schema_version",
	"name",
	"version",
	"description",
	"mcp_server",
	"tools",
	"install",
}

func TestPluginJSONExists(t *testing.T) {
	// Arrange / Act / Assert
	if _, err := os.Stat("../plugin.json"); os.IsNotExist(err) {
		t.Fatal("plugin.json not found in repo root")
	}
}

func TestPluginJSONIsValidJSON(t *testing.T) {
	// Arrange
	data, err := os.ReadFile("../plugin.json")
	if err != nil {
		t.Fatalf("read plugin.json: %v", err)
	}

	// Act
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("plugin.json is not valid JSON: %v", err)
	}

	// Assert: required fields present
	for _, field := range requiredPluginFields {
		if _, ok := m[field]; !ok {
			t.Errorf("plugin.json missing required field %q", field)
		}
	}
}

func TestPluginJSONMCPServer(t *testing.T) {
	// Arrange
	data, _ := os.ReadFile("../plugin.json")
	var m map[string]any
	json.Unmarshal(data, &m)

	// Act
	srv, ok := m["mcp_server"].(map[string]any)
	if !ok {
		t.Fatal("mcp_server must be an object")
	}

	// Assert
	if _, ok := srv["command"]; !ok {
		t.Error("mcp_server missing 'command' field")
	}
	if _, ok := srv["args"]; !ok {
		t.Error("mcp_server missing 'args' field")
	}
}

func TestPluginJSONVersionMatchesCode(t *testing.T) {
	data, err := os.ReadFile("../plugin.json")
	if err != nil {
		t.Fatalf("read plugin.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("plugin.json invalid JSON: %v", err)
	}
	got, _ := m["version"].(string)
	if got != version.Current {
		t.Errorf("plugin.json version %q != internal/version.Current %q â€” update one to match the other", got, version.Current)
	}
}

func TestPluginJSONToolsNonEmpty(t *testing.T) {
	// Arrange
	data, _ := os.ReadFile("../plugin.json")
	var m map[string]any
	json.Unmarshal(data, &m)

	// Act
	tools, ok := m["tools"].([]any)
	if !ok {
		t.Fatal("tools must be an array")
	}

	// Assert: at least 8 tools (all MCP tools)
	if len(tools) < 8 {
		t.Errorf("want at least 8 tools, got %d", len(tools))
	}
	for i, tool := range tools {
		tm, ok := tool.(map[string]any)
		if !ok {
			t.Errorf("tools[%d] is not an object", i)
			continue
		}
		if _, ok := tm["name"]; !ok {
			t.Errorf("tools[%d] missing 'name'", i)
		}
		if _, ok := tm["description"]; !ok {
			t.Errorf("tools[%d] missing 'description'", i)
		}
	}
}
