// Package engram handles detection and installation of the engram MCP server.
package engram

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Jomruizgo/Engrafo/v2/internal/version"
)

// State describes the current engram installation.
type State struct {
	Found      bool
	Path       string
	Version    string // bare semver, e.g. "1.16.3"
	Compatible bool   // detected >= pinned (same major, minor >=)
	Newer      bool   // detected > pinned (user upgraded beyond tested version)
}

// Detect checks whether engram is in PATH and returns its version state.
func Detect() State {
	path, err := exec.LookPath("engram")
	if err != nil {
		return State{}
	}

	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		// Found but cannot determine version â€” treat as incompatible.
		return State{Found: true, Path: path}
	}

	ver := parseVersion(string(out))
	compat, newer := checkCompatibility(ver)
	return State{
		Found:      true,
		Path:       path,
		Version:    ver,
		Compatible: compat,
		Newer:      newer,
	}
}

// EnsureCompatible installs the pinned engram version if absent or outdated.
// If the user has a newer version it prints a warning but does not downgrade.
// Returns nil on success; a non-nil error if installation was needed but failed.
func EnsureCompatible(w io.Writer) error {
	s := Detect()

	switch {
	case s.Found && s.Compatible && s.Newer:
		fmt.Fprintf(w, "  [WARN] engram v%s â€” newer than tested (%s); compatibility not guaranteed\n",
			s.Version, version.EngramCompatible)
		return nil

	case s.Found && s.Compatible:
		fmt.Fprintf(w, "  [OK]   engram v%s\n", s.Version)
		return nil

	case s.Found && !s.Compatible:
		fmt.Fprintf(w, "  [--]   engram v%s â€” older than tested (%s); upgrading...\n",
			s.Version, version.EngramCompatible)
		return install(w)

	default:
		fmt.Fprintf(w, "  [--]   engram not found â€” installing %s...\n", version.EngramCompatible)
		return install(w)
	}
}

// install runs `go install github.com/Gentleman-Programming/engram@{EngramCompatible}`.
// If go is not in PATH it prints manual install instructions instead.
func install(w io.Writer) error {
	goExe, err := exec.LookPath("go")
	if err != nil {
		fmt.Fprintf(w, "  [FAIL] go not found in PATH â€” install engram manually:\n")
		fmt.Fprintf(w, "         brew install gentleman-programming/tap/engram\n")
		fmt.Fprintf(w, "         scoop install engram\n")
		fmt.Fprintf(w, "         go install github.com/Gentleman-Programming/engram@%s\n",
			version.EngramCompatible)
		return fmt.Errorf("go not in PATH â€” install engram manually")
	}

	pkg := "github.com/Gentleman-Programming/engram@" + version.EngramCompatible
	cmd := exec.Command(goExe, "install", pkg)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go install engram: %w", err)
	}

	fmt.Fprintf(w, "  [OK]   engram %s installed\n", version.EngramCompatible)
	return nil
}

// parseVersion extracts the bare semver from engram --version output.
// Expected format: "engram v1.16.3"
func parseVersion(output string) string {
	s := strings.TrimSpace(output)
	if idx := strings.LastIndex(s, " "); idx >= 0 {
		s = s[idx+1:]
	}
	return strings.TrimPrefix(s, "v")
}

// checkCompatibility returns (compatible, newer) relative to EngramCompatible.
// compatible = detected major matches pinned and detected minor >= pinned minor.
// newer      = detected is strictly newer than pinned.
func checkCompatibility(detected string) (compatible, newer bool) {
	pinned := strings.TrimPrefix(version.EngramCompatible, "v")
	dMaj, dMin, dPatch := splitSemver(detected)
	pMaj, pMin, pPatch := splitSemver(pinned)

	if dMaj != pMaj {
		return false, dMaj > pMaj
	}
	if dMin != pMin {
		return dMin > pMin, dMin > pMin
	}
	return true, dPatch > pPatch
}

func splitSemver(v string) (major, minor, patch int) {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) >= 1 {
		major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		patch, _ = strconv.Atoi(parts[2])
	}
	return
}
