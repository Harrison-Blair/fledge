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
	register("pulse", runPulse, "fledge pulse <name> [--json]")
}

// pulseReport is the --json shape: the liveness classification alongside
// the declared quiet period and elapsed time that produced it. Reporting
// both halves (FC-7) is the only check on an implausible --expect
// declaration, since PLM-035 deliberately caps nothing.
type pulseReport struct {
	Name    string `json:"name"`
	Stalled bool   `json:"stalled"`
	Reason  string `json:"reason"`
	Expect  string `json:"expect,omitempty"`
	Elapsed string `json:"elapsed,omitempty"`
}

// classifyPulse turns a status record (or its absence) into a pulseReport.
// All liveness logic is ledger.ClassifyLiveness's; this function is glue —
// decoding the record and computing elapsed for display — never a
// staleness decision of its own (AC-8). now is passed explicitly, not read
// from the wall clock, so tests can pin it exactly (the awaitClock
// convention).
//
// No record (rec == nil) is a distinct third state: not stalled, with a
// reason naming the absence, never routed through ClassifyLiveness — a
// worker that hasn't heartbeat yet is starting up, not dead.
func classifyPulse(name string, rec *ledger.Record, now time.Time) (pulseReport, error) {
	if rec == nil {
		return pulseReport{
			Name:    name,
			Stalled: false,
			Reason:  fmt.Sprintf("no status record for %s: worker has not reported in yet", name),
		}, nil
	}
	var status ledger.StatusRecord
	if err := rec.Decode(&status); err != nil {
		return pulseReport{}, err
	}
	updatedAt, err := time.Parse(time.RFC3339, status.UpdatedAt)
	if err != nil {
		return pulseReport{}, fmt.Errorf("status record for %s has invalid updated_at %q: %w", name, status.UpdatedAt, err)
	}
	expect, err := time.ParseDuration(status.Expect)
	if err != nil {
		return pulseReport{}, fmt.Errorf("status record for %s has invalid expect %q: %w", name, status.Expect, err)
	}
	stalled, reason := ledger.ClassifyLiveness(updatedAt, expect, now)
	return pulseReport{
		Name:    name,
		Stalled: stalled,
		Reason:  reason,
		Expect:  status.Expect,
		Elapsed: now.Sub(updatedAt).String(),
	}, nil
}

func runPulse(args []string) int {
	fs := flag.NewFlagSet("pulse", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 {
		return usageErr("usage: fledge pulse <name> [--json]")
	}
	name := positional[0]

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	rec, err := ledger.Read(r.LedgerDir(), name, ledger.KindStatus)
	if err != nil {
		var notFound *ledger.NotFoundError
		var invalid *ledger.InvalidSubjectError
		switch {
		case errors.As(err, &notFound):
			rec = nil
		case errors.As(err, &invalid):
			return usageErr("%v", err)
		default:
			return fail("%v", err)
		}
	}

	report, err := classifyPulse(name, rec, time.Now())
	if err != nil {
		return fail("%v", err)
	}

	if *jsonOut {
		return emitJSON(report)
	}
	if report.Expect != "" {
		fmt.Printf("pulse %s: stalled=%t (%s) — declared %s, elapsed %s\n",
			report.Name, report.Stalled, report.Reason, report.Expect, report.Elapsed)
	} else {
		fmt.Printf("pulse %s: stalled=%t (%s)\n", report.Name, report.Stalled, report.Reason)
	}
	return ExitOK
}
