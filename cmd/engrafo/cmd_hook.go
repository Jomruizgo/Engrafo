package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
	"github.com/Jomruizgo/Engrafo/v2/internal/hooks"
	"github.com/Jomruizgo/Engrafo/v2/internal/workspace"
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
