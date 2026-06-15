package main

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/Jomruizgo/Engrafo/v2/internal/engram"
	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
	"github.com/Jomruizgo/Engrafo/v2/internal/hooks"
	"github.com/Jomruizgo/Engrafo/v2/internal/workspace"
)

// hookEvent is the JSON payload Claude Code sends to hook stdin.
type hookEvent struct {
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     map[string]any  `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
}

func cmdHook(cfg *config, sub []string) error {
	if len(sub) == 0 {
		return fmt.Errorf("hook: specify subcommand (session-start | pre-read | pre-write)")
	}
	switch sub[0] {
	case "session-start":
		return hookSessionStart(cfg)
	case "pre-read":
		return hookPreRead(cfg)
	case "pre-write":
		return hookPreWrite(cfg)
	case "pre-compact":
		return hookPreCompact(cfg)
	case "post-mem-save":
		return hookPostMemSave(cfg)
	default:
		return fmt.Errorf("hook: unknown subcommand %q", sub[0])
	}
}

// hookOpenStore opens the graph store without erroring on failure.
// Returns nil when the store cannot be opened (hook must not block).
func hookOpenStore(cfg *config) *graph.Store {
	dbPath, err := cfg.resolveDB()
	if err != nil {
		return nil
	}
	s, err := graph.Open(dbPath)
	if err != nil {
		return nil
	}
	return s
}

func hookSessionStart(cfg *config) error {
	var msg string
	s := hookOpenStore(cfg)
	if s != nil {
		defer s.Close()

		// Disparar update en proceso (best-effort; salida silenciada para no corromper el JSON).
		if roots, err := s.AllRoots(); err == nil && len(roots) > 0 {
			silentCfg := &config{stdout: io.Discard}
			p := newParser()
			_ = runUpdate(silentCfg, s, p, roots)
		}

		msg = hooks.SessionStartMessage(graph.NewQuerier(s))
	}
	fmt.Fprint(cfg.stdout, hooks.BuildOutput(msg))
	return nil
}

func hookPreRead(cfg *config) error {
	ev := decodeHookEvent(cfg.stdin)
	filePath := toolInputFilePath(ev)

	var msg string
	if filePath != "" {
		s := hookOpenStore(cfg)
		if s != nil {
			defer s.Close()
			q := graph.NewQuerier(s)
			rootName, relPath, ok := workspace.ResolveFileToRoot(s, filePath)
			if ok {
				msg = hooks.PreReadMessage(q, relPath, rootName)
			} else {
				msg = hooks.PreReadMessage(q, filePath, "")
			}
		}
	}
	fmt.Fprint(cfg.stdout, hooks.BuildOutput(msg))
	return nil
}

func hookPreWrite(cfg *config) error {
	ev := decodeHookEvent(cfg.stdin)
	filePath := toolInputFilePath(ev)

	var msg string
	if filePath != "" {
		s := hookOpenStore(cfg)
		if s != nil {
			defer s.Close()
			q := graph.NewQuerier(s)
			rootName, relPath, ok := workspace.ResolveFileToRoot(s, filePath)
			if ok {
				msg = hooks.PreWriteMessage(q, relPath, 3, rootName)
			} else {
				msg = hooks.PreWriteMessage(q, filePath, 3, "")
			}
		}
	}
	fmt.Fprint(cfg.stdout, hooks.BuildOutput(msg))
	return nil
}

func hookPreCompact(cfg *config) error {
	s := hookOpenStore(cfg)
	var q *graph.Querier
	if s != nil {
		defer s.Close()
		q = graph.NewQuerier(s)
	}
	msg := hooks.PreCompactMessage(q)
	fmt.Fprint(cfg.stdout, hooks.BuildOutput(msg))
	return nil
}

// obsIDPattern matches engram observation sync IDs, e.g. "obs-3fd0bf535828d470".
var obsIDPattern = regexp.MustCompile(`obs-[0-9a-zA-Z_-]+`)

// hookPostMemSave fires after engram's mem_save. It extracts the new observation's
// sync_id from the tool response and auto-anchors it to every graph node whose
// symbol is mentioned in the saved title/content. This is the bridge that makes
// engrafo + engram a single system: memories become queryable from cg_node/cg_history.
func hookPostMemSave(cfg *config) error {
	ev := decodeHookEvent(cfg.stdin)
	if ev == nil {
		fmt.Fprint(cfg.stdout, hooks.BuildOutput(""))
		return nil
	}

	obsID := extractObsID(ev.ToolResponse)
	text := memSaveText(ev.ToolInput)
	if obsID == "" || text == "" {
		fmt.Fprint(cfg.stdout, hooks.BuildOutput(""))
		return nil
	}

	s := hookOpenStore(cfg)
	if s == nil {
		fmt.Fprint(cfg.stdout, hooks.BuildOutput(""))
		return nil
	}
	defer s.Close()

	n, err := engram.New(s).AutoAnchor(obsID, text, "")
	var msg string
	if err == nil && n > 0 {
		msg = fmt.Sprintf("[engrafo] memoria %s anclada a %d simbolo(s) del grafo", obsID, n)
	}
	fmt.Fprint(cfg.stdout, hooks.BuildOutput(msg))
	return nil
}

// extractObsID finds the engram observation sync_id in the raw tool response.
// Claude Code may deliver the MCP result nested (content[].text holding JSON),
// so we scan the serialized payload rather than assume a fixed shape.
func extractObsID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	return obsIDPattern.FindString(string(raw))
}

// memSaveText concatenates the title and content args of a mem_save call.
func memSaveText(input map[string]any) string {
	if input == nil {
		return ""
	}
	var parts []string
	for _, key := range []string{"title", "content", "message", "body"} {
		if v, ok := input[key].(string); ok && v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

func decodeHookEvent(r io.Reader) *hookEvent {
	if r == nil {
		return nil
	}
	var ev hookEvent
	if err := json.NewDecoder(r).Decode(&ev); err != nil {
		return nil
	}
	return &ev
}

func toolInputFilePath(ev *hookEvent) string {
	if ev == nil || ev.ToolInput == nil {
		return ""
	}
	fp, _ := ev.ToolInput["file_path"].(string)
	return fp
}
