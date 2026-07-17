package cli

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/Harrison-Blair/fledge/internal/ledger"
	"github.com/Harrison-Blair/fledge/internal/repo"
)

func init() {
	register("heartbeat", runHeartbeat, "fledge heartbeat <name> [--note <text>] [--expect <duration>] [--json]")
}

func runHeartbeat(args []string) int {
	fs := flag.NewFlagSet("heartbeat", flag.ContinueOnError)
	note := fs.String("note", "", "short free-text status note")
	expectStr := fs.String("expect", "", "declared quiet period, e.g. 12m (default: 5m)")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 {
		return usageErr("usage: fledge heartbeat <name> [--note <text>] [--expect <duration>]")
	}
	name := positional[0]

	// expectDeclared is what gets stored: the caller's own text when given
	// (so "12m" reads back as "12m", not a round-tripped "12m0s"), or the
	// StaleAfter default rendered explicitly so the record stays
	// self-describing for every reader, including one who never passed
	// --expect.
	expectDeclared := ledger.StaleAfter.String()
	if *expectStr != "" {
		if _, err := time.ParseDuration(*expectStr); err != nil {
			return usageErr("invalid --expect %q: %v", *expectStr, err)
		}
		expectDeclared = *expectStr
	}

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	payload := ledger.StatusRecord{
		Note:      *note,
		Expect:    expectDeclared,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	rec, err := ledger.Write(r.LedgerDir(), name, ledger.KindStatus, payload)
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
	fmt.Printf("heartbeat %s at %s\n", name, rec.Timestamp)
	return ExitOK
}
