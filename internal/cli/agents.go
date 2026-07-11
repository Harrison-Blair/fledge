package cli

import (
	"flag"
	"fmt"
	"sort"

	"github.com/Harrison-Blair/fledge/internal/bootstrap"
	"github.com/Harrison-Blair/fledge/internal/repo"
)

func init() { register("agents", runAgents, "fledge agents [--json]") }

// adapterInfo is the JSON shape for one adapter in `fledge agents`/`--list-agents`.
type adapterInfo struct {
	Name       string `json:"name"`
	Tier       string `json:"tier"`
	Detector   string `json:"detector"`
	Scaffolded bool   `json:"scaffolded"`
}

func runAgents(args []string) int {
	fs := flag.NewFlagSet("agents", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}

	adapters, err := bootstrap.LoadAdapters()
	if err != nil {
		return fail("%v", err)
	}
	sort.Slice(adapters, func(i, j int) bool { return adapters[i].Name < adapters[j].Name })

	// Scaffolded status needs a repo; tolerate running outside one.
	root := ""
	if r, err := repo.Find(); err == nil {
		root = r.Root
	}

	out := make([]adapterInfo, 0, len(adapters))
	for _, m := range adapters {
		out = append(out, manifestInfo(m, root != "" && m.Scaffolded(root)))
	}

	if *jsonOut {
		return emitJSON(map[string]any{"agents": out})
	}
	fmt.Println("agent adapters (tier derived from primitive coverage):")
	for _, a := range out {
		mark := "not scaffolded"
		if a.Scaffolded {
			mark = "scaffolded"
		}
		fmt.Printf("  %s\ttier %s\tdetector %s\t%s\n", a.Name, tierLabel(a.Tier), a.Detector, mark)
	}
	return ExitOK
}

// manifestInfo builds the adapterInfo for a manifest.
func manifestInfo(m *bootstrap.Manifest, scaffolded bool) adapterInfo {
	return adapterInfo{
		Name:       m.Name,
		Tier:       tierLabel(m.Tier()),
		Detector:   m.Detector.Exists,
		Scaffolded: scaffolded,
	}
}
