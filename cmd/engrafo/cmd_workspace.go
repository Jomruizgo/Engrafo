package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
	"github.com/Jomruizgo/Engrafo/v2/internal/workspace"
)

// cmdWorkspace maneja los subcomandos workspace add/list/remove.
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
		return fmt.Errorf("subcomando desconocido %q â€” usa: add, list, remove", args[0])
	}
}

// cmdWorkspaceAdd agrega una raÃ­z al manifest y la indexa inmediatamente.
func cmdWorkspaceAdd(cfg *config, args []string) error {
	fs2 := flag.NewFlagSet("workspace add", flag.ContinueOnError)
	remote := fs2.String("remote", "", "URL remote del repo")
	branch := fs2.String("branch", "", "rama por defecto")
	vcsFlag := fs2.String("vcs", "", "sistema de control de versiones: git|none")
	fs2.SetOutput(cfg.stdout)
	if err := fs2.Parse(args); err != nil {
		return err
	}
	rest := fs2.Args()
	if len(rest) < 2 {
		return fmt.Errorf("uso: engrafo workspace add <name> <path> [--remote URL] [--branch B] [--vcs git|none]")
	}
	name := rest[0]
	rootPath := rest[1]

	dbPath, err := cfg.resolveDB()
	if err != nil {
		return err
	}

	manifestPath, wsDir := cfg.manifestPath, cfg.workspaceDir
	if manifestPath == "" {
		cwd, _ := os.Getwd()
		wsDir = cwd
		manifestPath = filepath.Join(cwd, "engrafo.json")
		if _, statErr := os.Stat(manifestPath); os.IsNotExist(statErr) {
			if saveErr := workspace.Save(manifestPath, &workspace.Manifest{Version: 1}); saveErr != nil {
				return fmt.Errorf("crear engrafo.json: %w", saveErr)
			}
			fmt.Fprintf(cfg.stdout, "creado %s\n", manifestPath)
		}
	}

	m, err := workspace.Load(manifestPath)
	if err != nil {
		return fmt.Errorf("cargar manifest: %w", err)
	}

	for _, r := range m.Roots {
		if r.Name == name {
			return fmt.Errorf("ya existe una raÃ­z con nombre %q", name)
		}
	}

	spec := workspace.RootSpec{
		Name:   name,
		Path:   rootPath,
		Remote: *remote,
		Branch: *branch,
		VCS:    *vcsFlag,
	}
	resolved, err := spec.Resolve(wsDir)
	if err != nil {
		return fmt.Errorf("resolver raÃ­z: %w", err)
	}

	m.Roots = append(m.Roots, spec)
	if err := workspace.Save(manifestPath, m); err != nil {
		return fmt.Errorf("guardar manifest: %w", err)
	}

	s, err := graph.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer s.Close()

	if err := initRoot(cfg, s, resolved); err != nil {
		return fmt.Errorf("indexar %s: %w", name, err)
	}

	fmt.Fprintf(cfg.stdout, "raÃ­z %q agregada y indexada (%s)\n", name, resolved.AbsRoot)
	return nil
}

// cmdWorkspaceList muestra las raÃ­ces registradas con estadÃ­sticas bÃ¡sicas.
func cmdWorkspaceList(cfg *config) error {
	dbPath, err := cfg.resolveDB()
	if err != nil {
		return err
	}
	s, err := graph.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer s.Close()

	roots, err := s.AllRoots()
	if err != nil {
		return fmt.Errorf("all roots: %w", err)
	}
	if len(roots) == 0 {
		fmt.Fprintln(cfg.stdout, "(sin raÃ­ces â€” ejecuta 'engrafo workspace add' o 'engrafo init')")
		return nil
	}

	w := tabwriter.NewWriter(cfg.stdout, 2, 8, 2, ' ', 0)
	fmt.Fprintln(w, "NOMBRE\tRUTA\tVCS\tREMOTE\tNODOS\tINDEXADO")
	for _, r := range roots {
		var nodeCount int
		s.DB().QueryRow(
			`SELECT COUNT(*) FROM nodes WHERE root_id=? AND kind!='external'`, r.ID,
		).Scan(&nodeCount)

		remote := r.RemoteURL
		if remote == "" {
			remote = "(sin remote)"
		}
		indexed := r.IndexedAt
		if indexed == "" {
			indexed = "â€”"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
			r.Name, r.RelPath, r.VCS, remote, nodeCount, indexed)
	}
	w.Flush()
	return nil
}

// cmdWorkspaceRemove elimina una raÃ­z del manifest y de la DB (cascade).
func cmdWorkspaceRemove(cfg *config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: engrafo workspace remove <name>")
	}
	name := args[0]

	dbPath, err := cfg.resolveDB()
	if err != nil {
		return err
	}

	if cfg.manifestPath != "" {
		m, err := workspace.Load(cfg.manifestPath)
		if err != nil {
			return fmt.Errorf("cargar manifest: %w", err)
		}
		filtered := m.Roots[:0]
		found := false
		for _, r := range m.Roots {
			if r.Name == name {
				found = true
				continue
			}
			filtered = append(filtered, r)
		}
		if !found {
			return fmt.Errorf("raÃ­z %q no encontrada en el manifest", name)
		}
		m.Roots = filtered
		if err := workspace.Save(cfg.manifestPath, m); err != nil {
			return fmt.Errorf("guardar manifest: %w", err)
		}
	}

	s, err := graph.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer s.Close()

	var nodeCount int
	s.DB().QueryRow(`
		SELECT COUNT(*) FROM nodes n
		JOIN roots r ON r.id = n.root_id
		WHERE r.name = ?`, name).Scan(&nodeCount)

	res, err := s.DB().Exec(`DELETE FROM roots WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete root: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("raÃ­z %q no encontrada en la DB", name)
	}

	fmt.Fprintf(cfg.stdout, "raÃ­z %q eliminada (%d nodos borrados)\n", name, nodeCount)
	return nil
}
