package cli

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/repo"
	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() { register("init", runInit, "fledge init [--json]") }

//go:embed scan-ignore.default
var defaultScanIgnore []byte

// gitignore lines fledge needs; appended as one block when any is missing.
var gitignoreLines = []string{".fledge/context/raw/", ".fledge/locks/"}

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}

	var created, skipped []string
	note := func(rel string, wasCreated bool) {
		if wasCreated {
			created = append(created, rel)
		} else {
			skipped = append(skipped, rel)
		}
	}

	files := []struct {
		rel     string
		content []byte
	}{
		{".fledge/context/raw/.gitkeep", nil},
		{".fledge/locks/.gitkeep", nil},
		{".fledge/scan-ignore", defaultScanIgnore},
		{"spec/requirements/.gitkeep", nil},
		{"spec/tasks/.gitkeep", nil},
	}
	for _, f := range files {
		path := filepath.Join(r.Root, f.rel)
		if fileExists(path) {
			note(f.rel, false)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fail("%v", err)
		}
		if err := spec.WriteFileAtomic(path, f.content); err != nil {
			return fail("%v", err)
		}
		note(f.rel, true)
	}

	changed, err := ensureGitignore(filepath.Join(r.Root, ".gitignore"))
	if err != nil {
		return fail("%v", err)
	}
	note(".gitignore", changed)

	if *jsonOut {
		if created == nil {
			created = []string{}
		}
		if skipped == nil {
			skipped = []string{}
		}
		return emitJSON(map[string][]string{"created": created, "skipped": skipped})
	}
	for _, rel := range created {
		fmt.Printf("created %s\n", rel)
	}
	for _, rel := range skipped {
		fmt.Printf("exists %s\n", rel)
	}
	return ExitOK
}

// ensureGitignore appends fledge's ignore lines when missing; reports change.
func ensureGitignore(path string) (bool, error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	have := map[string]bool{}
	for _, l := range strings.Split(string(existing), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var missing []string
	for _, l := range gitignoreLines {
		if !have[l] {
			missing = append(missing, l)
		}
	}
	if len(missing) == 0 {
		return false, nil
	}
	var b strings.Builder
	b.Write(existing)
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("# fledge — per-run intermediates, regenerable\n")
	for _, l := range missing {
		b.WriteString(l + "\n")
	}
	return true, spec.WriteFileAtomic(path, []byte(b.String()))
}
