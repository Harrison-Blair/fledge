// Package repo locates the git repository root and fledge's directories
// within it.
package repo

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo is a resolved fledge working repository.
type Repo struct {
	Root string // absolute path to the git worktree root
}

// Find locates the enclosing git repository from the current directory.
func Find() (*Repo, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, errors.New("not inside a git repository")
	}
	return &Repo{Root: strings.TrimSpace(string(out))}, nil
}

func (r *Repo) FledgeDir() string       { return filepath.Join(r.Root, ".fledge") }
func (r *Repo) LocksDir() string        { return filepath.Join(r.FledgeDir(), "broods") }
func (r *Repo) ContextDir() string      { return filepath.Join(r.FledgeDir(), "nest") }
func (r *Repo) ScanIgnorePath() string  { return filepath.Join(r.Root, ".fledgeignore") }
func (r *Repo) EvidenceDir() string     { return filepath.Join(r.FledgeDir(), "molt") }
func (r *Repo) RequirementsDir() string { return filepath.Join(r.Root, "pluma", "plumage") }
func (r *Repo) TasksDir() string        { return filepath.Join(r.Root, "pluma", "feathers") }

// RequireFledge errors unless .fledge/ exists at the repo root.
func (r *Repo) RequireFledge() error {
	info, err := os.Stat(r.FledgeDir())
	if err != nil || !info.IsDir() {
		return fmt.Errorf(".fledge/ not found at %s (run `fledge init`)", r.Root)
	}
	return nil
}

// Version returns the repo's VERSION file contents, or fallback when absent.
func (r *Repo) Version(fallback string) string {
	b, err := os.ReadFile(filepath.Join(r.Root, "VERSION"))
	if err != nil {
		return fallback
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		return fallback
	}
	return v
}

// Head returns the full HEAD commit sha, or "" when the repo has no commits.
func (r *Repo) Head() string {
	out, err := exec.Command("git", "-C", r.Root, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
