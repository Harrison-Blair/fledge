package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Harrison-Blair/fledge/internal/bootstrap"
	"github.com/Harrison-Blair/fledge/internal/repo"
)

func init() {
	register("dev", runDev, "fledge dev status [--json]")
}

// runDev dispatches on the first positional argument, mirroring runNest's
// per-verb FlagSet pattern (internal/cli/nest.go).
func runDev(args []string) int {
	if len(args) == 0 {
		return usageErr("usage: fledge dev status ...")
	}
	verb := args[0]
	switch verb {
	case "status":
		return runDevStatus(args[1:])
	default:
		return usageErr("fledge dev: unknown verb %q (available: status)", verb)
	}
}

// devStatusJSON is the --json shape for `fledge dev status` (Q6/FC-8/FC-9).
type devStatusJSON struct {
	Linked bool     `json:"linked"`
	Source string   `json:"source"`
	Count  int      `json:"count"`
	Broken []string `json:"broken"`
}

// runDevStatus reports whether the repo is dev-linked (PLM-031), per FTHR-077's
// stamp, and names any dev-linked path whose target no longer resolves. The
// stamp is the sole source of truth — this never walks the tree guessing dev
// state from what happens to be a symlink. Exit code: ExitOK when not
// dev-linked or dev-linked with no broken links; ExitFail when any link is
// broken (a finding, not a crash, mirroring `preen`).
func runDevStatus(args []string) int {
	fs := flag.NewFlagSet("dev status", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if _, err := parseMixed(fs, args); err != nil {
		return ExitUsage
	}

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}

	// LoadStamp returns (nil, nil) when .fledge/scaffold.json is absent — a
	// repo with no stamp at all is "not dev-linked", not an error.
	stamp, err := bootstrap.LoadStamp(r.Root)
	if err != nil {
		return fail("%v", err)
	}

	if stamp == nil || stamp.DevSource == "" {
		if *jsonOut {
			return emitJSON(devStatusJSON{Broken: []string{}})
		}
		fmt.Println("not dev-linked")
		return ExitOK
	}

	count := 0
	var broken []string
	for path, entry := range stamp.Files {
		if entry.Policy != "dev-link" {
			continue
		}
		count++
		if _, statErr := os.Stat(filepath.Join(r.Root, filepath.FromSlash(path))); statErr != nil {
			broken = append(broken, path)
		}
	}
	sort.Strings(broken)

	out := devStatusJSON{
		Linked: true,
		Source: stamp.DevSource,
		Count:  count,
		Broken: broken,
	}
	if out.Broken == nil {
		out.Broken = []string{}
	}

	if *jsonOut {
		if code := emitJSON(out); code != ExitOK {
			return code
		}
	} else {
		fmt.Printf("dev-linked: source=%s files=%d\n", out.Source, out.Count)
		if len(broken) > 0 {
			fmt.Println("broken links:")
			for _, b := range broken {
				fmt.Printf("  %s\n", b)
			}
		}
	}

	if len(broken) > 0 {
		return ExitFail
	}
	return ExitOK
}
