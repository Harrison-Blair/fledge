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
	register("await", runAwait,
		"fledge await <subject> --kind <kind> --timeout <duration> [--exists] [--json]\n"+
			"  wait mode: verdict/escalation kinds use --exists (has it landed?); status kind uses change-wait (has it changed?)")
}

// awaitPollInterval is the fixed polling interval for fledge await, matching
// the 1-2s range agreed during PLM-030 interrogation.
const awaitPollInterval = 1 * time.Second

// awaitClock is the injectable time source for the polling loop, so its
// logic can be covered by fast unit tests instead of real sleeps.
type awaitClock struct {
	now   func() time.Time
	sleep func(time.Duration)
}

func realAwaitClock() awaitClock {
	return awaitClock{now: time.Now, sleep: time.Sleep}
}

// awaitResult is what pollAwait found: the record that satisfied the wait
// (nil on timeout with no appearance ever observed), and whether the wait
// ended because --timeout elapsed rather than an appearance/change.
type awaitResult struct {
	record   *ledger.Record
	timedOut bool
}

// pollAwait polls dir for (subject, kind) via read. In existence-wait
// (exists true) it returns as soon as a record is present, consulting
// neither its payload nor its timestamp and sampling no baseline — the
// property that makes it immune to both the baseline race and the
// identical-payload rewrite defect. Otherwise (change-wait) it returns when
// the record first appears or its payload changes from the baseline sampled
// at call time. In either mode, when hasTimeout, timeout elapsing ends the
// wait. A non-NotFoundError from read is returned immediately rather than
// polled through.
func pollAwait(read func(dir, subject, kind string) (*ledger.Record, error), clock awaitClock, dir, subject, kind string, exists bool, timeout time.Duration, hasTimeout bool) (awaitResult, error) {
	present := true
	var baselinePayload string
	if !exists {
		baseline, err := read(dir, subject, kind)
		if err != nil {
			var notFound *ledger.NotFoundError
			if !errors.As(err, &notFound) {
				return awaitResult{}, err
			}
			present = false
		} else {
			baselinePayload = string(baseline.Payload)
		}
	}

	deadline := clock.now().Add(timeout)
	var last *ledger.Record
	for {
		rec, err := read(dir, subject, kind)
		if err != nil {
			var notFound *ledger.NotFoundError
			if !errors.As(err, &notFound) {
				return awaitResult{}, err
			}
			rec = nil
		}
		last = rec
		if exists {
			if rec != nil {
				return awaitResult{record: rec}, nil
			}
		} else if rec != nil && (!present || string(rec.Payload) != baselinePayload) {
			return awaitResult{record: rec}, nil
		}

		sleepDur := awaitPollInterval
		if hasTimeout {
			now := clock.now()
			if !now.Before(deadline) {
				return awaitResult{record: last, timedOut: true}, nil
			}
			if remaining := deadline.Sub(now); remaining < sleepDur {
				sleepDur = remaining
			}
		}
		clock.sleep(sleepDur)
	}
}

// awaitEnvelope is the --json shape: the record (or null), plus an explicit
// timed_out field on the timeout path only (omitted, per this codebase's
// omitempty convention, on success).
type awaitEnvelope struct {
	Record   *ledger.Record `json:"record"`
	TimedOut bool           `json:"timed_out,omitempty"`
}

func runAwait(args []string) int {
	fs := flag.NewFlagSet("await", flag.ContinueOnError)
	kind := fs.String("kind", "", "ledger record kind (required)")
	timeoutStr := fs.String("timeout", "", "maximum time to block, e.g. 200ms, 5s (required)")
	exists := fs.Bool("exists", false, "existence-wait: return as soon as the record exists, ignoring payload and timestamp")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 || *kind == "" {
		return usageErr("usage: fledge await <subject> --kind <kind> --timeout <duration> [--exists] [--json]")
	}
	if *timeoutStr == "" {
		return usageErr("--timeout is required: usage: fledge await <subject> --kind <kind> --timeout <duration> [--exists] [--json]")
	}
	subject := positional[0]

	timeout, err := time.ParseDuration(*timeoutStr)
	if err != nil {
		return usageErr("invalid --timeout %q: %v", *timeoutStr, err)
	}

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	result, err := pollAwait(ledger.Read, realAwaitClock(), r.LedgerDir(), subject, *kind, *exists, timeout, true)
	if err != nil {
		var invalid *ledger.InvalidSubjectError
		if errors.As(err, &invalid) {
			return usageErr("%v", err)
		}
		return fail("%v", err)
	}

	if *jsonOut {
		if emitJSON(awaitEnvelope{Record: result.record, TimedOut: result.timedOut}) != ExitOK {
			return ExitFail
		}
		if result.timedOut {
			return ExitTimeout
		}
		return ExitOK
	}
	if result.timedOut {
		fmt.Printf("await %s timed out waiting for a %s record\n", subject, *kind)
		return ExitTimeout
	}
	fmt.Printf("await %s: record updated at %s\n", subject, result.record.Timestamp)
	return ExitOK
}
