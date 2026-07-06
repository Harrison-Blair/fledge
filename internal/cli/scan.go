package cli

import (
	"flag"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/repo"
	"github.com/Harrison-Blair/fledge/internal/scan"
)

func init() { register("scan", runScan, "fledge scan [--json]") }

func runScan(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}
	res, err := scan.Run(r.Root)
	if err != nil {
		return fail("%v", err)
	}
	if *jsonOut {
		out := map[string]any{"modules": res.Modules}
		if res.Commit == "" {
			out["commit"] = nil
		} else {
			out["commit"] = res.Commit
		}
		if res.Modules == nil {
			out["modules"] = []scan.Module{}
		}
		return emitJSON(out)
	}
	// Byte-compatible with the retired .fledge/scripts/scan output.
	fmt.Printf("# fledge scan — commit %s\n\n", res.ShortCommit)
	for _, m := range res.Modules {
		fmt.Printf("module: %s (files: %d, bytes: %d)\n", m.Name, m.Count, m.Bytes)
		for _, f := range m.Files {
			fmt.Printf("  %s\n", f)
		}
		fmt.Println()
	}
	return ExitOK
}
