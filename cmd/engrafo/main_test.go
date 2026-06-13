package main

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
)

func seedTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	s, err := graph.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	b := graph.NewBuilder(s)
	if err := b.UpsertFile("init", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "user", Kind: "package", FilePath: "user.go", Language: "go"},
			{Symbol: "UserService", Kind: "class", FilePath: "user.go", Language: "go"},
		},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	s.Close()
	return dbPath
}

func TestRunNoArgs(t *testing.T) {
	// Arrange / Act / Assert
	var buf bytes.Buffer
	if err := runWith(nil, nil, &buf); err != nil {
		t.Fatalf("want nil for no args, got %v", err)
	}
	if !strings.Contains(buf.String(), "engrafo") {
		t.Errorf("usage must mention 'engrafo', got %q", buf.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	// Arrange / Act / Assert
	if err := runWith([]string{"bogus"}, nil, io.Discard); err == nil {
		t.Error("want error for unknown command, got nil")
	}
}

func TestHookSessionStartOutputsJSON(t *testing.T) {
	// Arrange
	dbPath := seedTestDB(t)
	stdin := strings.NewReader(`{"hook_event_name":"SessionStart"}`)
	var out bytes.Buffer

	// Act
	if err := runWith([]string{"--db", dbPath, "hook", "session-start"}, stdin, &out); err != nil {
		t.Fatalf("hook session-start returned unexpected error: %v", err)
	}

	// Assert
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("output is not valid JSON: %s", out.String())
	}
}

func TestHookPreReadOutputsJSON(t *testing.T) {
	// Arrange
	dbPath := seedTestDB(t)
	stdin := strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":"user.go"}}`)
	var out bytes.Buffer

	// Act
	if err := runWith([]string{"--db", dbPath, "hook", "pre-read"}, stdin, &out); err != nil {
		t.Fatalf("hook pre-read returned unexpected error: %v", err)
	}

	// Assert
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("output is not valid JSON: %s", out.String())
	}
}

func TestHookPreWriteOutputsJSON(t *testing.T) {
	// Arrange
	dbPath := seedTestDB(t)
	stdin := strings.NewReader(`{"tool_name":"Edit","tool_input":{"file_path":"user.go"}}`)
	var out bytes.Buffer

	// Act
	if err := runWith([]string{"--db", dbPath, "hook", "pre-write"}, stdin, &out); err != nil {
		t.Fatalf("hook pre-write returned unexpected error: %v", err)
	}

	// Assert
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("output is not valid JSON: %s", out.String())
	}
}

func TestHookNeverErrors(t *testing.T) {
	// Hook commands must never error even when db is missing — exit code always 0.
	// Arrange
	stdin := strings.NewReader(`{}`)
	var out bytes.Buffer

	// Act
	err := runWith([]string{"--db", "/nonexistent/graph.db", "hook", "session-start"}, stdin, &out)

	// Assert
	if err != nil {
		t.Fatalf("hook must never error (always exit 0), got: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("hook must always output valid JSON, got: %s", out.String())
	}
}

func TestDoctorFailsWithoutDB(t *testing.T) {
	// Arrange / Act / Assert
	var buf bytes.Buffer
	if err := runWith([]string{"--db", "/nonexistent/graph.db", "doctor"}, nil, &buf); err == nil {
		t.Error("want error from doctor when db missing, got nil")
	}
}

func TestStatusFailsWithoutDB(t *testing.T) {
	// Arrange / Act / Assert
	var buf bytes.Buffer
	if err := runWith([]string{"--db", "/nonexistent/graph.db", "status"}, nil, &buf); err == nil {
		t.Error("want error from status when db missing, got nil")
	}
}

func TestInitFromGitFailsOutsideRepo(t *testing.T) {
	// Arrange: temp dir is not a git repository
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	var buf bytes.Buffer

	// Act: pass dir explicitly as root so init doesn't fallback to "." (repo CWD)
	err := runWith([]string{"--db", dbPath, "init", "--from-git", "5", dir}, nil, &buf)

	// Assert: returns error (not a git repo)
	if err == nil {
		t.Error("want error for init --from-git outside git repo, got nil")
	}
}
