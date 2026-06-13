package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/hooks"
)

// hookEvent is the JSON payload Claude Code sends to hook stdin.
type hookEvent struct {
	HookEventName string         `json:"hook_event_name"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
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
			msg = hooks.PreReadMessage(graph.NewQuerier(s), filePath)
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
			msg = hooks.PreWriteMessage(graph.NewQuerier(s), filePath, 3)
		}
	}
	fmt.Fprint(cfg.stdout, hooks.BuildOutput(msg))
	return nil
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
