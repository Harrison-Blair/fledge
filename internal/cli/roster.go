package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/repo"
	"github.com/Harrison-Blair/fledge/internal/roster"
)

func init() {
	register("roster", runRoster,
		"fledge roster [--json] | roster assign --feather FTHR-### [--pair] [--json] | roster release <name> [--json]")
}

// runRoster dispatches on the first positional argument: assign, release, or —
// when absent or a flag (e.g. `roster --json`) — the list default. Mirrors
// runNest's per-subcommand FlagSet pattern.
func runRoster(args []string) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runRosterList(args)
	}
	switch args[0] {
	case "assign":
		return runRosterAssign(args[1:])
	case "release":
		return runRosterRelease(args[1:])
	default:
		return usageErr("fledge roster: unknown verb %q (available: assign, release, or none for list)", args[0])
	}
}

func runRosterAssign(args []string) int {
	fs := flag.NewFlagSet("roster assign", flag.ContinueOnError)
	feather := fs.String("feather", "", "feather ID the allocation is for (required)")
	pair := fs.Bool("pair", false, "allocate a brooder+skua pair sharing one species")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if _, err := parseMixed(fs, args); err != nil {
		return ExitUsage
	}
	if *feather == "" {
		return usageErr("fledge roster assign: --feather is required")
	}

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	tokens, err := roster.Assign(r.RosterDir(), *feather, *pair)
	if err != nil {
		return fail("%v", err)
	}
	names := composeNames(tokens, *pair)

	if *jsonOut {
		return emitJSON(map[string][]string{"names": names})
	}
	for _, n := range names {
		fmt.Println(n)
	}
	return ExitOK
}

func runRosterRelease(args []string) int {
	fs := flag.NewFlagSet("roster release", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) == 0 {
		return usageErr("usage: fledge roster release <name>")
	}
	name := positional[0]

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	if err := roster.Release(r.RosterDir(), speciesToken(name)); err != nil {
		return fail("%v", err)
	}

	if *jsonOut {
		return emitJSON(map[string]string{"released": name})
	}
	fmt.Printf("released %s\n", name)
	return ExitOK
}

func runRosterList(args []string) int {
	fs := flag.NewFlagSet("roster", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if _, err := parseMixed(fs, args); err != nil {
		return ExitUsage
	}

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	entries, err := roster.List(r.RosterDir())
	if err != nil {
		return fail("%v", err)
	}

	if *jsonOut {
		if entries == nil {
			entries = []roster.Entry{}
		}
		return emitJSON(entries)
	}
	if len(entries) == 0 {
		fmt.Println("no roster assignments")
		return ExitOK
	}
	for _, e := range entries {
		fmt.Printf("%s\t%s\n", e.Species, e.Feather)
	}
	return ExitOK
}

// rolePrefixes are the fixed <role>- prefixes composed onto a shared species for
// a pair. speciesToken strips one so `roster release fledge-brooder-adelie`
// reaches internal/roster's species-token space.
var rolePrefixes = []string{"fledge-brooder-", "fledge-skua-"}

// composeNames builds the caller-facing name(s) from the allocated species
// token(s). A pair is always brooder+skua sharing the one species; a solo
// allocation returns the bare species token for the caller to prefix.
func composeNames(tokens []string, pair bool) []string {
	if pair && len(tokens) == 2 {
		return []string{"fledge-brooder-" + tokens[0], "fledge-skua-" + tokens[1]}
	}
	return tokens
}

// speciesToken reduces a caller-facing name to its species token by stripping a
// known role prefix, leaving a bare species token untouched.
func speciesToken(name string) string {
	for _, p := range rolePrefixes {
		if strings.HasPrefix(name, p) {
			return strings.TrimPrefix(name, p)
		}
	}
	return name
}
