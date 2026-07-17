package cli

import (
	"errors"
	"flag"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/ledger"
	"github.com/Harrison-Blair/fledge/internal/repo"
)

func init() {
	register("ledger", runLedger, "fledge ledger read <subject> --kind status|verdict|escalation [--json]")
}

// runLedger dispatches on the first positional argument, mirroring
// runRoster's per-verb dispatch pattern.
func runLedger(args []string) int {
	if len(args) == 0 {
		return usageErr("fledge ledger: a verb is required (available: read)")
	}
	switch args[0] {
	case "read":
		return runLedgerRead(args[1:])
	default:
		return usageErr("fledge ledger: unknown verb %q (available: read)", args[0])
	}
}

func runLedgerRead(args []string) int {
	fs := flag.NewFlagSet("ledger read", flag.ContinueOnError)
	kind := fs.String("kind", "", "ledger record kind (required)")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 || *kind == "" {
		return usageErr("usage: fledge ledger read <subject> --kind status|verdict|escalation")
	}
	subject := positional[0]

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	rec, err := ledger.Read(r.LedgerDir(), subject, *kind)
	if err != nil {
		var invalid *ledger.InvalidSubjectError
		if errors.As(err, &invalid) {
			return usageErr("%v", err)
		}
		return fail("%v", err)
	}
	if *jsonOut {
		return emitJSON(rec)
	}
	fmt.Printf("%s %s at %s: %s\n", rec.Subject, rec.Kind, rec.Timestamp, rec.Payload)
	return ExitOK
}
