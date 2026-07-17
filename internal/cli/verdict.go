package cli

import (
	"errors"
	"flag"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/ledger"
	"github.com/Harrison-Blair/fledge/internal/repo"
)

func init() {
	register("verdict", runVerdict, "fledge verdict <subject> --result pass|fail [--note <text>] [--json]")
}

func runVerdict(args []string) int {
	fs := flag.NewFlagSet("verdict", flag.ContinueOnError)
	result := fs.String("result", "", "verdict outcome: pass or fail (required)")
	note := fs.String("note", "", "short free-text note")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 {
		return usageErr("usage: fledge verdict <subject> --result pass|fail [--note <text>]")
	}
	if *result != "pass" && *result != "fail" {
		return usageErr("usage: fledge verdict <subject> --result pass|fail: got --result %q", *result)
	}
	subject := positional[0]

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	payload := ledger.VerdictRecord{Result: *result, Note: *note}
	rec, err := ledger.Write(r.LedgerDir(), subject, ledger.KindVerdict, payload)
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
	fmt.Printf("verdict %s: %s at %s\n", subject, *result, rec.Timestamp)
	return ExitOK
}
