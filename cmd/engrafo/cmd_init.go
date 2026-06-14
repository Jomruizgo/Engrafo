package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Jomruizgo/Engrafo/internal/graph"
	"github.com/Jomruizgo/Engrafo/internal/parser"
	"github.com/Jomruizgo/Engrafo/internal/parser/extractors"
)

// skipDirs are directory names that are never indexed.
var skipDirs = map[string]bool{
	".git": true, ".engrafo": true, "node_modules": true,
	"vendor": true, ".idea": true, ".vscode": true,
	"dist": true, "build": true, "__pycache__": true,
	"target": true, ".next": true, ".nuxt": true,
}

func newParser() *parser.Parser {
	return parser.New(
		&extractors.GoExtractor{},
		&extractors.TypeScriptExtractor{},
		&extractors.PythonExtractor{},
	)
}

func cmdInit(cfg *config, args []string) error {
	fs2 := flag.NewFlagSet("init", flag.ContinueOnError)
	fromGit := fs2.Int("from-git", 0, "build bi-temporal history from last N git commits")
	fs2.SetOutput(cfg.stdout)
	if err := fs2.Parse(args); err != nil {
		return err
	}

	root := "."
	if rest := fs2.Args(); len(rest) > 0 {
		root = rest[0]
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	dbPath, dbErr := cfg.resolveDB()
	if dbErr != nil {
		dbPath = filepath.Join(root, ".engrafo", "graph.db")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("create .engrafo dir: %w", err)
	}

	if *fromGit > 0 {
		return initFromGit(cfg, root, dbPath, *fromGit)
	}
	return initFull(cfg, root, dbPath)
}

// initFull indexes the current working tree (default init behavior).
func initFull(cfg *config, root, dbPath string) error {
	s, err := graph.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer s.Close()

	commitHash := currentHEAD(root)
	vcs := detectVCS(root)

	rootID, err := s.UpsertRoot(graph.ResolvedRoot{
		Name:    filepath.Base(root),
		RelPath: ".",
		AbsRoot: root,
		VCS:     vcs,
	})
	if err != nil {
		return fmt.Errorf("upsert root: %w", err)
	}

	revSource := "git"
	revHash := commitHash
	if commitHash == "init" || commitHash == "" {
		revSource = "init"
		revHash = ""
	}
	revID, err := s.CreateRevision(rootID, revSource, revHash)
	if err != nil {
		return fmt.Errorf("create revision: %w", err)
	}

	p := newParser()
	b := graph.NewBuilder(s)

	var indexed, skipped int
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}

		lang := parser.Detect(path)
		if lang == "" {
			return nil
		}

		result, parseErr := p.ParseFile(path)
		if parseErr != nil {
			skipped++
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for i := range result.Nodes {
			result.Nodes[i].FilePath = rel
		}

		if upsertErr := b.UpsertFile(rootID, revID, "", result); upsertErr != nil {
			return fmt.Errorf("upsert %s: %w", rel, upsertErr)
		}
		indexed++
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	db := s.DB()
	db.Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('last_commit_hash',?)`, commitHash)
	db.Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('repo_root',?)`, root)
	db.Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('indexed_at',datetime('now'))`)
	s.SetRootIndexed(rootID, commitHash)

	fmt.Fprintf(cfg.stdout, "indexed %d files (%d skipped) — db: %s\n", indexed, skipped, dbPath)
	return nil
}

// initFromGit builds the bi-temporal graph by replaying the last N git commits.
// Commits are processed oldest-first so that edge invalidation is correct.
func initFromGit(cfg *config, root, dbPath string, n int) error {
	// Require git repo
	if _, err := exec.Command("git", "-C", root, "rev-parse", "--git-dir").Output(); err != nil {
		return fmt.Errorf("init --from-git: not a git repository at %s", root)
	}

	// Get last N commits (newest first)
	hashesOut, err := exec.Command("git", "-C", root,
		"log", "--format=%H", fmt.Sprintf("-%d", n)).Output()
	if err != nil {
		return fmt.Errorf("git log: %w", err)
	}
	hashes := strings.Fields(strings.TrimSpace(string(hashesOut)))
	if len(hashes) == 0 {
		return fmt.Errorf("no commits found")
	}

	// Reverse: process oldest first
	for i, j := 0, len(hashes)-1; i < j; i, j = i+1, j-1 {
		hashes[i], hashes[j] = hashes[j], hashes[i]
	}

	s, err := graph.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer s.Close()

	rootID, err := s.UpsertRoot(graph.ResolvedRoot{
		Name:    filepath.Base(root),
		RelPath: ".",
		AbsRoot: root,
		VCS:     "git",
	})
	if err != nil {
		return fmt.Errorf("upsert root: %w", err)
	}

	p := newParser()
	b := graph.NewBuilder(s)

	var totalFiles int
	for i, hash := range hashes {
		revID, err := s.CreateRevision(rootID, "git", hash)
		if err != nil {
			return fmt.Errorf("create revision for %s: %w", hash, err)
		}

		var prevHash string
		if i > 0 {
			prevHash = hashes[i-1]
		}

		changed := changedFiles(root, hash, prevHash)
		for _, relPath := range changed {
			if parser.Detect(relPath) == "" {
				continue
			}

			// Read file content at this commit via git show
			content, showErr := exec.Command("git", "-C", root, "show", hash+":"+relPath).Output()
			if showErr != nil {
				continue // file deleted in this commit
			}

			result, parseErr := p.ParseContent(relPath, content)
			if parseErr != nil {
				continue
			}

			norm := filepath.ToSlash(relPath)
			for j := range result.Nodes {
				result.Nodes[j].FilePath = norm
			}

			b.UpsertFile(rootID, revID, "", result)
			totalFiles++
		}

		fmt.Fprintf(cfg.stdout, "  [%d/%d] %s\n", i+1, len(hashes), hash[:12])
	}

	headHash := hashes[len(hashes)-1]
	db := s.DB()
	db.Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('last_commit_hash',?)`, headHash)
	db.Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('repo_root',?)`, root)
	db.Exec(`INSERT OR REPLACE INTO index_meta(key,value) VALUES('indexed_at',datetime('now'))`)
	s.SetRootIndexed(rootID, headHash)

	fmt.Fprintf(cfg.stdout, "replayed %d commits, %d file-versions — db: %s\n",
		len(hashes), totalFiles, dbPath)
	return nil
}

// changedFiles returns the files changed in hash relative to prevHash.
// When prevHash is empty, returns all files introduced in hash.
func changedFiles(root, hash, prevHash string) []string {
	var out []byte
	if prevHash == "" {
		out, _ = exec.Command("git", "-C", root,
			"diff-tree", "--no-commit-id", "-r", "--name-only", hash).Output()
	} else {
		out, _ = exec.Command("git", "-C", root,
			"diff", "--name-only", prevHash+".."+hash).Output()
	}
	return strings.Fields(strings.TrimSpace(string(out)))
}

func currentHEAD(repoRoot string) string {
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return "init"
	}
	return strings.TrimSpace(string(out))
}

func detectVCS(root string) string {
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		return "git"
	}
	return "none"
}
