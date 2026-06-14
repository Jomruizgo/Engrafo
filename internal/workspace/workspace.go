// Package workspace parses engrafo.json manifests and resolves root specs to absolute paths.
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
)

// Manifest representa el archivo engrafo.json del workspace.
type Manifest struct {
	Version int        `json:"version"`
	Roots   []RootSpec `json:"roots"`
}

// RootSpec es una entrada del array "roots" en el manifest.
type RootSpec struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Remote string `json:"remote,omitempty"`
	Branch string `json:"branch,omitempty"`
	VCS    string `json:"vcs,omitempty"`
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// Resolve convierte un RootSpec en un ResolvedRoot con todas las rutas absolutas,
// el VCS detectado y remote/branch resueltos desde git cuando aplica.
// manifestDir es el directorio que contiene el engrafo.json.
func (s RootSpec) Resolve(manifestDir string) (graph.ResolvedRoot, error) {
	if !validName.MatchString(s.Name) {
		return graph.ResolvedRoot{}, fmt.Errorf("nombre de raÃ­z invÃ¡lido %q: debe coincidir con ^[a-zA-Z0-9._-]+$", s.Name)
	}
	if s.Path == "" {
		return graph.ResolvedRoot{}, fmt.Errorf("raÃ­z %q: path es obligatorio", s.Name)
	}

	absRoot := s.Path
	if !filepath.IsAbs(absRoot) {
		absRoot = filepath.Join(manifestDir, absRoot)
	}
	absRoot = filepath.Clean(absRoot)

	if _, err := os.Stat(absRoot); err != nil {
		return graph.ResolvedRoot{}, fmt.Errorf("raÃ­z %q: el directorio %q no existe", s.Name, absRoot)
	}

	// rel_path: path del manifest (relativo o ".") que apunta a esta raÃ­z.
	relPath := s.Path
	if filepath.IsAbs(relPath) {
		relPath = "."
	}

	// Detectar VCS si no estÃ¡ explÃ­cito.
	vcs := s.VCS
	if vcs == "" {
		if _, err := os.Stat(filepath.Join(absRoot, ".git")); err == nil {
			vcs = "git"
		} else {
			vcs = "none"
		}
	}

	// Resolver remote desde git si no se especificÃ³.
	remote := s.Remote
	if remote == "" && vcs == "git" {
		if out, err := exec.Command("git", "-C", absRoot, "remote", "get-url", "origin").Output(); err == nil {
			remote = strings.TrimSpace(string(out))
		}
	}

	// Resolver branch desde git si no se especificÃ³.
	branch := s.Branch
	if branch == "" && vcs == "git" {
		if out, err := exec.Command("git", "-C", absRoot, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
			branch = strings.TrimSpace(string(out))
		}
	}

	return graph.ResolvedRoot{
		Name:          s.Name,
		RelPath:       relPath,
		AbsRoot:       absRoot,
		RemoteURL:     remote,
		DefaultBranch: branch,
		VCS:           vcs,
	}, nil
}

// Load lee y valida un engrafo.json en la ruta dada.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leer manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsear manifest: %w", err)
	}
	if err := validateManifest(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Save escribe el manifest en path con indentaciÃ³n de 2 espacios.
func Save(path string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// validateManifest verifica la integridad del manifest: nombres vÃ¡lidos y Ãºnicos, paths presentes.
func validateManifest(m *Manifest) error {
	seen := make(map[string]bool, len(m.Roots))
	for _, r := range m.Roots {
		if !validName.MatchString(r.Name) {
			return fmt.Errorf("nombre de raÃ­z invÃ¡lido %q: debe coincidir con ^[a-zA-Z0-9._-]+$", r.Name)
		}
		if seen[r.Name] {
			return fmt.Errorf("nombre de raÃ­z duplicado %q", r.Name)
		}
		seen[r.Name] = true
		if r.Path == "" {
			return fmt.Errorf("raÃ­z %q: path es obligatorio", r.Name)
		}
	}
	return nil
}

// FindManifest busca engrafo.json subiendo desde startDir hasta la raÃ­z del filesystem.
// Devuelve la ruta al manifest, el directorio que lo contiene y true si se encontrÃ³.
func FindManifest(startDir string) (manifestPath, wsDir string, found bool) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, "engrafo.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

// FindGitRoot busca .git subiendo desde startDir hasta la raÃ­z del filesystem.
// Devuelve el directorio que contiene .git y true si se encontrÃ³.
func FindGitRoot(startDir string) (string, bool) {
	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
