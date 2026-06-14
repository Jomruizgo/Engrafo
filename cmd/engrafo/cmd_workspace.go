package main

import "fmt"

// cmdWorkspace maneja los subcomandos de workspace (add/list/remove).
// Implementación completa en Fase 4.
func cmdWorkspace(cfg *config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: engrafo workspace <add|list|remove> [args]")
	}
	switch args[0] {
	case "add":
		return cmdWorkspaceAdd(cfg, args[1:])
	case "list":
		return cmdWorkspaceList(cfg)
	case "remove":
		return cmdWorkspaceRemove(cfg, args[1:])
	default:
		return fmt.Errorf("subcomando desconocido %q — usa: add, list, remove", args[0])
	}
}

func cmdWorkspaceAdd(_ *config, _ []string) error {
	return fmt.Errorf("workspace add: no implementado aún (Fase 4)")
}

func cmdWorkspaceList(_ *config) error {
	return fmt.Errorf("workspace list: no implementado aún (Fase 4)")
}

func cmdWorkspaceRemove(_ *config, _ []string) error {
	return fmt.Errorf("workspace remove: no implementado aún (Fase 4)")
}
