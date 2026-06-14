package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
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
	rootID, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "test", RelPath: ".", AbsRoot: dir, VCS: "none",
	})
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}
	revID, err := s.CreateRevision(rootID, "init", "")
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	b := graph.NewBuilder(s)
	if err := b.UpsertFile(rootID, revID, "", &parser.Result{
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
	var buf bytes.Buffer
	if err := runWith(nil, nil, &buf); err != nil {
		t.Fatalf("want nil for no args, got %v", err)
	}
	if !strings.Contains(buf.String(), "engrafo") {
		t.Errorf("usage must mention 'engrafo', got %q", buf.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	if err := runWith([]string{"bogus"}, nil, io.Discard); err == nil {
		t.Error("want error for unknown command, got nil")
	}
}

func TestHookSessionStartOutputsJSON(t *testing.T) {
	dbPath := seedTestDB(t)
	stdin := strings.NewReader(`{"hook_event_name":"SessionStart"}`)
	var out bytes.Buffer

	if err := runWith([]string{"--db", dbPath, "hook", "session-start"}, stdin, &out); err != nil {
		t.Fatalf("hook session-start returned unexpected error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("output is not valid JSON: %s", out.String())
	}
}

func TestHookPreReadOutputsJSON(t *testing.T) {
	dbPath := seedTestDB(t)
	stdin := strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":"user.go"}}`)
	var out bytes.Buffer

	if err := runWith([]string{"--db", dbPath, "hook", "pre-read"}, stdin, &out); err != nil {
		t.Fatalf("hook pre-read returned unexpected error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("output is not valid JSON: %s", out.String())
	}
}

func TestHookPreWriteOutputsJSON(t *testing.T) {
	dbPath := seedTestDB(t)
	stdin := strings.NewReader(`{"tool_name":"Edit","tool_input":{"file_path":"user.go"}}`)
	var out bytes.Buffer

	if err := runWith([]string{"--db", dbPath, "hook", "pre-write"}, stdin, &out); err != nil {
		t.Fatalf("hook pre-write returned unexpected error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("output is not valid JSON: %s", out.String())
	}
}

func TestHookNeverErrors(t *testing.T) {
	stdin := strings.NewReader(`{}`)
	var out bytes.Buffer

	err := runWith([]string{"--db", "/nonexistent/graph.db", "hook", "session-start"}, stdin, &out)

	if err != nil {
		t.Fatalf("hook must never error (always exit 0), got: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("hook must always output valid JSON, got: %s", out.String())
	}
}

func TestDoctorFailsWithoutDB(t *testing.T) {
	var buf bytes.Buffer
	if err := runWith([]string{"--db", "/nonexistent/graph.db", "doctor"}, nil, &buf); err == nil {
		t.Error("want error from doctor when db missing, got nil")
	}
}

func TestStatusFailsWithoutDB(t *testing.T) {
	var buf bytes.Buffer
	if err := runWith([]string{"--db", "/nonexistent/graph.db", "status"}, nil, &buf); err == nil {
		t.Error("want error from status when db missing, got nil")
	}
}

func TestInitFromGitFailsOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	var buf bytes.Buffer

	err := runWith([]string{"--db", dbPath, "init", "--from-git", "5", dir}, nil, &buf)

	if err == nil {
		t.Error("want error for init --from-git outside git repo, got nil")
	}
}

// TestInitAutoRegistersGitRoot — test #3: init en directorio con .git → raíz vcs=git auto-registrada.
func TestInitAutoRegistersGitRoot(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".engrafo", "graph.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}

	// Solo el directorio .git es necesario para que detectVCS retorne "git".
	writeGitDir(t, dir)

	var buf bytes.Buffer
	if err := runWith([]string{"--db", dbPath, "init", dir}, nil, &buf); err != nil {
		t.Fatalf("init falló: %v\noutput: %s", err, buf.String())
	}

	s, err := graph.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer s.Close()

	roots, err := s.AllRoots()
	if err != nil {
		t.Fatalf("all roots: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("want 1 root, got %d", len(roots))
	}
	if roots[0].VCS != "git" {
		t.Errorf("want vcs=git, got %q", roots[0].VCS)
	}
	if roots[0].Name != filepath.Base(dir) {
		t.Errorf("want name=%q, got %q", filepath.Base(dir), roots[0].Name)
	}

	// Sin commits reales → currentHEAD devuelve "init" → revision source='init'.
	var src string
	if err := s.DB().QueryRow(
		`SELECT source FROM revisions WHERE root_id=?`, roots[0].ID,
	).Scan(&src); err != nil {
		t.Fatalf("query revision: %v", err)
	}
	if src != "init" {
		t.Errorf("want revision source=init, got %q", src)
	}
}

// TestVcsNoneUpdateInvalidatesDeletedFile — test #4: vcs=none, update detecta archivo borrado
// y crea revisión checksum invalidando sus aristas.
func TestVcsNoneUpdateInvalidatesDeletedFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".engrafo", "graph.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}

	// Sembrar DB directamente (sin parser, CGO_ENABLED=0 no parsea .go).
	s, err := graph.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	rootID, err := s.UpsertRoot(graph.ResolvedRoot{
		Name: "myproject", RelPath: ".", AbsRoot: dir, VCS: "none",
	})
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}

	initRevID, err := s.CreateRevision(rootID, "init", "")
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}

	b := graph.NewBuilder(s)
	// Seed: main.go llama a dos símbolos → 2 aristas activas.
	// main.go NO existe en disco → update lo detecta como borrado.
	if err := b.UpsertFile(rootID, initRevID, "fakechecksum", &parser.Result{
		Nodes: []parser.Node{
			{Symbol: "doWork", Kind: "function", FilePath: "main.go", Language: "go"},
			{Symbol: "helper", Kind: "function", FilePath: "helper.go", Language: "go"},
		},
		Edges: []parser.Edge{
			{FromSymbol: "main.go", ToSymbol: "doWork", Kind: "calls"},
			{FromSymbol: "main.go", ToSymbol: "helper", Kind: "calls"},
		},
	}); err != nil {
		t.Fatalf("UpsertFile: %v", err)
	}
	s.SetRootIndexed(rootID, "")
	s.Close()

	// Confirmar estado inicial: 2 aristas activas.
	s2, _ := graph.Open(dbPath)
	var activeInit int
	s2.DB().QueryRow(`SELECT COUNT(*) FROM edges WHERE valid_until_rev IS NULL`).Scan(&activeInit)
	if activeInit != 2 {
		t.Fatalf("want 2 active edges before update, got %d", activeInit)
	}
	s2.Close()

	// CLI: update — main.go no está en disco → detectado como borrado.
	var buf bytes.Buffer
	if err := runWith([]string{"--db", dbPath, "update"}, nil, &buf); err != nil {
		t.Fatalf("update falló: %v\noutput: %s", err, buf.String())
	}

	s3, _ := graph.Open(dbPath)
	defer s3.Close()

	// Deben existir 2 revisiones: init + checksum.
	var revCount int
	s3.DB().QueryRow(`SELECT COUNT(*) FROM revisions WHERE root_id=?`, rootID).Scan(&revCount)
	if revCount != 2 {
		t.Errorf("want 2 revisions (init + checksum), got %d", revCount)
	}

	// Segunda revisión debe ser source='checksum'.
	var csSource string
	s3.DB().QueryRow(
		`SELECT source FROM revisions WHERE root_id=? ORDER BY id DESC LIMIT 1`, rootID,
	).Scan(&csSource)
	if csSource != "checksum" {
		t.Errorf("want 2nd revision source=checksum, got %q", csSource)
	}

	// Las aristas de main.go deben estar invalidadas.
	var activeAfter int
	s3.DB().QueryRow(`SELECT COUNT(*) FROM edges WHERE valid_until_rev IS NULL`).Scan(&activeAfter)
	if activeAfter != 0 {
		t.Errorf("want 0 active edges after main.go deleted, got %d", activeAfter)
	}

	// valid_until_rev de las aristas debe apuntar a la revisión checksum.
	var csRevID int64
	s3.DB().QueryRow(
		`SELECT id FROM revisions WHERE root_id=? AND source='checksum' LIMIT 1`, rootID,
	).Scan(&csRevID)
	var invalidatedCount int
	s3.DB().QueryRow(
		`SELECT COUNT(*) FROM edges WHERE valid_until_rev=?`, csRevID,
	).Scan(&invalidatedCount)
	if invalidatedCount != 2 {
		t.Errorf("want 2 edges invalidated with csRevID=%d, got %d", csRevID, invalidatedCount)
	}
}
