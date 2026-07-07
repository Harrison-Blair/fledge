package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/check"
)

func init() { register("preen", runCheck, "fledge preen [--strict] [--json]") }

func runCheck(args []string) int {
	fs := flag.NewFlagSet("preen", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	strict := fs.Bool("strict", false, "treat warnings as errors")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	r, set, locked, code, ok := loadSet()
	if !ok {
		return code
	}
	findings := check.Run(set, locked, r.EvidenceDir())
	for i := range findings {
		findings[i].File = relPath(r.Root, findings[i].File)
	}
	failed := check.HasErrors(findings) || (*strict && len(findings) > 0)

	if *jsonOut {
		if findings == nil {
			findings = []check.Finding{}
		}
		emitJSON(map[string]any{"ok": !failed, "findings": findings})
	} else {
		errs, warns := 0, 0
		for _, f := range findings {
			label := "WARN "
			if f.Severity == check.Error {
				label = "ERROR"
				errs++
			} else {
				warns++
			}
			fmt.Printf("%s %s: %s\n", label, f.File, f.Message)
		}
		switch {
		case len(findings) == 0:
			fmt.Printf("spec clean: %d plumages, %d feathers\n", len(set.Reqs), len(set.Tasks))
		default:
			fmt.Printf("%s\n", summaryLine(errs, warns))
		}
	}
	if failed {
		return ExitFail
	}
	return ExitOK
}

func summaryLine(errs, warns int) string {
	var parts []string
	if errs > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", errs))
	}
	if warns > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", warns))
	}
	return strings.Join(parts, ", ")
}
