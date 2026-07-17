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
	register("await", runAwait, "fledge await <subject> --kind <kind> [--timeout <duration>] [--json]")
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

// pollAwait polls dir for (subject, kind) via read until the record first
// appears, its payload changes from the baseline observed at call time, or
// (when hasTimeout) timeout elapses. A non-NotFoundError from read is
// returned immediately rather than polled through.
func pollAwait(read func(dir, subject, kind string) (*ledger.Record, error), clock awaitClock, dir, subject, kind string, timeout time.Duration, hasTimeout bool) (awaitResult, error) {
	baseline, err := read(dir, subject, kind)
	present := true
	var baselinePayload string
	if err != nil {
		var notFound *ledger.NotFoundError
		if !errors.As(err, &notFound) {
			return awaitResult{}, err
		}
		present = false
	} else {
		baselinePayload = string(baseline.Payload)
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
		if rec != nil && (!present || string(rec.Payload) != baselinePayload) {
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
	timeoutStr := fs.String("timeout", "", "maximum time to block, e.g. 200ms, 5s (default: block indefinitely)")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 || *kind == "" {
		return usageErr("usage: fledge await <subject> --kind <kind> [--timeout <duration>]")
	}
	subject := positional[0]

	var timeout time.Duration
	hasTimeout := *timeoutStr != ""
	if hasTimeout {
		timeout, err = time.ParseDuration(*timeoutStr)
		if err != nil {
			return usageErr("invalid --timeout %q: %v", *timeoutStr, err)
		}
	}

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	result, err := pollAwait(ledger.Read, realAwaitClock(), r.LedgerDir(), subject, *kind, timeout, hasTimeout)
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
