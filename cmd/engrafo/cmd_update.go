package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
)

func cmdUpdate(cfg *config, args []string) error {
	fs2 := flag.NewFlagSet("update", flag.ContinueOnError)
	rootFilter := fs2.String("root", "", "actualizar solo esta raíz (por nombre)")
	fs2.SetOutput(cfg.stdout)
	if err := fs2.Parse(args); err != nil {
		return err
	}

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
		return fmt.Errorf("no hay raíces indexadas — ejecuta 'engrafo init' primero")
	}

	p := newParser()
	for _, root := range roots {
		if *rootFilter != "" && root.Name != *rootFilter {
			continue
		}
		switch root.VCS {
		case "git":
			if err := updateGitRoot(cfg, s, root, p); err != nil {
				fmt.Fprintf(cfg.stdout, "  [%s] error: %v\n", root.Name, err)
			}
		default: // "none"
			if err := updateChecksumRoot(cfg, s, root, p); err != nil {
				fmt.Fprintf(cfg.stdout, "  [%s] error: %v\n", root.Name, err)
			}
		}
	}
	return nil
}

// updateGitRoot actualiza una raíz vcs=git mediante git diff.
func updateGitRoot(cfg *config, s *graph.Store, root graph.RootRow, p *parser.Parser) error {
	headOut, err := exec.Command("git", "-C", root.AbsRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	currentCommit := strings.TrimSpace(string(headOut))

	lastCommit := root.LastCommitHash
	if lastCommit == "" {
		// Fallback para DBs migradas de v1 que aún usan index_meta.
		s.DB().QueryRow(`SELECT COALESCE(value,'') FROM index_meta WHERE key='last_commit_hash'`).Scan(&lastCommit)
	}

	if currentCommit == lastCommit {
		fmt.Fprintf(cfg.stdout, "[%s] ya está actualizado en %s\n", root.Name, currentCommit[:12])
		return nil
	}

	revID, err := s.CreateRevision(root.ID, "git", currentCommit)
	if err != nil {
		return fmt.Errorf("create revision: %w", err)
	}

	var diffOut []byte
	if lastCommit == "" {
		// Sin commit previo: diff-tree del HEAD para ver qué introdujo.
		diffOut, _ = exec.Command("git", "-C", root.AbsRoot,
			"diff-tree", "--no-commit-id", "-r", "--name-only", currentCommit).Output()
	} else {
		diffOut, err = exec.Command("git", "-C", root.AbsRoot, "diff", "--name-only",
			lastCommit+".."+currentCommit).Output()
		if err != nil {
			return fmt.Errorf("git diff: %w", err)
		}
	}

	changed := strings.Split(strings.TrimSpace(string(diffOut)), "\n")
	b := graph.NewBuilder(s)

	var updated, skipped int
	for _, relPath := range changed {
		if relPath == "" || parser.Detect(relPath) == "" {
			continue
		}
		absPath := filepath.Join(root.AbsRoot, relPath)
		result, parseErr := p.ParseFile(absPath)
		if parseErr != nil {
			skipped++
			continue
		}
		norm := filepath.ToSlash(relPath)
		for i := range result.Nodes {
			result.Nodes[i].FilePath = norm
		}
		if upsertErr := b.UpsertFile(root.ID, revID, "", result); upsertErr != nil {
			return fmt.Errorf("upsert %s: %w", relPath, upsertErr)
		}
		updated++
	}

	s.DB().Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('last_commit_hash',?)`, currentCommit)
	s.SetRootIndexed(root.ID, currentCommit)

	fmt.Fprintf(cfg.stdout, "[%s] actualizado %d archivos (%d omitidos) — en %s\n",
		root.Name, updated, skipped, currentCommit[:12])
	return nil
}

// updateChecksumRoot actualiza una raíz vcs=none comparando checksums sha256.
func updateChecksumRoot(cfg *config, s *graph.Store, root graph.RootRow, p *parser.Parser) error {
	type change struct {
		relPath  string
		deleted  bool
		content  []byte
		checksum string
	}
	var changes []change

	diskFiles := make(map[string]bool)
	_ = filepath.WalkDir(root.AbsRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if parser.Detect(path) == "" {
			return nil
		}
		rel, _ := filepath.Rel(root.AbsRoot, path)
		rel = filepath.ToSlash(rel)
		diskFiles[rel] = true

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		cs := sha256hex(content)
		stored, _ := s.FileChecksum(root.ID, rel)
		if stored != cs {
			changes = append(changes, change{rel, false, content, cs})
		}
		return nil
	})

	// Detectar archivos borrados (en DB pero no en disco).
	dbPaths, err := s.AllFilePaths(root.ID)
	if err != nil {
		return fmt.Errorf("all file paths: %w", err)
	}
	for _, dbPath := range dbPaths {
		if !diskFiles[dbPath] {
			changes = append(changes, change{dbPath, true, nil, ""})
		}
	}

	if len(changes) == 0 {
		fmt.Fprintf(cfg.stdout, "[%s] sin cambios\n", root.Name)
		return nil
	}

	revID, err := s.CreateRevision(root.ID, "checksum", "")
	if err != nil {
		return fmt.Errorf("create checksum revision: %w", err)
	}

	b := graph.NewBuilder(s)
	var updated, deletedCount int

	for _, ch := range changes {
		if ch.deleted {
			if invErr := b.InvalidateFile(root.ID, revID, ch.relPath); invErr != nil {
				fmt.Fprintf(cfg.stdout, "  [%s] error invalidating %s: %v\n", root.Name, ch.relPath, invErr)
			}
			deletedCount++
			continue
		}
		result, parseErr := p.ParseContent(ch.relPath, ch.content)
		if parseErr != nil {
			continue
		}
		for i := range result.Nodes {
			result.Nodes[i].FilePath = ch.relPath
		}
		if upsertErr := b.UpsertFile(root.ID, revID, ch.checksum, result); upsertErr != nil {
			fmt.Fprintf(cfg.stdout, "  [%s] error upserting %s: %v\n", root.Name, ch.relPath, upsertErr)
			continue
		}
		updated++
	}

	s.SetRootIndexed(root.ID, "")
	fmt.Fprintf(cfg.stdout, "[%s] %d actualizado(s), %d borrado(s) — revisión checksum\n",
		root.Name, updated, deletedCount)
	return nil
}

// sha256hex devuelve el sha256 del contenido en hex.
func sha256hex(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}
