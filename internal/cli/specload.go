package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/repo"
	"github.com/Harrison-Blair/fledge/internal/spec"
)

// loadSet resolves the repo, requires .fledge/, and loads all specs plus the
// IDs of tasks with a held lock. On failure it prints and returns a non-zero
// exit code as ok=false.
func loadSet() (r *repo.Repo, set *spec.Set, locked []string, exitCode int, ok bool) {
	r, err := repo.Find()
	if err != nil {
		return nil, nil, nil, envErr("%v", err), false
	}
	if err := r.RequireFledge(); err != nil {
		return nil, nil, nil, envErr("%v", err), false
	}
	set, err = spec.Load(r.RequirementsDir(), r.TasksDir())
	if err != nil {
		return nil, nil, nil, fail("%v", err), false
	}
	return r, set, lockedTaskIDs(r), 0, true
}

// lockedTaskIDs lists task IDs that have a lock file.
func lockedTaskIDs(r *repo.Repo) []string {
	matches, _ := filepath.Glob(filepath.Join(r.LocksDir(), "*.lock"))
	var ids []string
	for _, m := range matches {
		ids = append(ids, strings.TrimSuffix(filepath.Base(m), ".lock"))
	}
	return ids
}

// relPath makes p repo-relative for display; falls back to p unchanged.
func relPath(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return p
}

// fileExists reports whether path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
