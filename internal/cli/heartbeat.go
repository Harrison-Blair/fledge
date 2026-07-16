package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Harrison-Blair/fledge/internal/ledger"
	"github.com/Harrison-Blair/fledge/internal/repo"
)

func init() {
	register("heartbeat", runHeartbeat, "fledge heartbeat <name> [--note <text>] [--json]")
}

func runHeartbeat(args []string) int {
	fs := flag.NewFlagSet("heartbeat", flag.ContinueOnError)
	note := fs.String("note", "", "short free-text status note")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 {
		return usageErr("usage: fledge heartbeat <name> [--note <text>]")
	}
	name := positional[0]

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}

	payload := ledger.StatusRecord{
		PID:       os.Getpid(),
		Note:      *note,
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
