package workspace

import (
	"path/filepath"
	"strings"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
)

// ResolveFileToRoot mapea un path absoluto a (rootName, relPath) buscando en las raÃ­ces
// registradas la que tiene el prefijo abs_root mÃ¡s largo que sea ancestro del path.
// Devuelve ok=false si ningÃºn root matchea.
func ResolveFileToRoot(store *graph.Store, absFilePath string) (rootName, relPath string, ok bool) {
	roots, err := store.AllRoots()
	if err != nil || len(roots) == 0 {
		return "", "", false
	}

	// Normalizar separadores para comparaciÃ³n consistente.
	clean := filepath.Clean(absFilePath)

	bestLen := -1
	bestName := ""
	bestRoot := ""

	for _, r := range roots {
		absRoot := filepath.Clean(r.AbsRoot)
		if !strings.HasPrefix(clean, absRoot) {
			continue
		}
		// Asegurar que es realmente un ancestro (no un prefijo accidental de nombre).
		if len(clean) > len(absRoot) && clean[len(absRoot)] != filepath.Separator {
			continue
		}
		if len(absRoot) > bestLen {
			bestLen = len(absRoot)
			bestName = r.Name
			bestRoot = absRoot
		}
	}

	if bestLen < 0 {
		return "", "", false
	}

	rel, err := filepath.Rel(bestRoot, clean)
	if err != nil {
		return "", "", false
	}
	return bestName, filepath.ToSlash(rel), true
}
