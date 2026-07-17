package cli

import (
	"errors"
	"flag"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/ledger"
	"github.com/Harrison-Blair/fledge/internal/repo"
)

func init() {
	register("escalate", runEscalate, "fledge escalate <subject> --message <text> [--json]")
}

func runEscalate(args []string) int {
	fs := flag.NewFlagSet("escalate", flag.ContinueOnError)
	message := fs.String("message", "", "escalation message (required)")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 || *message == "" {
		return usageErr("usage: fledge escalate <subject> --message <text>")
	}
	subject := positional[0]

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	payload := ledger.EscalationRecord{Message: *message}
	rec, err := ledger.Write(r.LedgerDir(), subject, ledger.KindEscalation, payload)
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
	fmt.Printf("escalate %s at %s\n", subject, rec.Timestamp)
	return ExitOK
}
