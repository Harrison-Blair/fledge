// Package status implements fledge supervise status: reporting whether the
// session's supervision daemon is live, and what it watches.
package status

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/supervise"
)

// staleAfter is how old a heartbeat may be before the daemon looks hung; it
// writes one every 10 seconds.
const staleAfter = 30 * time.Second

// Options locates the repository from Cwd; Now dates the heartbeat.
type Options struct {
	Cwd string
	Now time.Time
}

// Result reports the daemon of Session. Stale marks a live daemon whose
// heartbeat is older than 30 seconds.
type Result struct {
	Session string            `json:"session"`
	Live    bool              `json:"live"`
	Stale   bool              `json:"stale"`
	Status  *supervise.Status `json:"status"`
}

// Run reads the daemon's lock. It never writes.
func Run(ctx context.Context, o Options) cli.Outcome {
	out := cli.NewOutcome("supervise.status")
	r, err := Read(ctx, o.Cwd, o.Now)
	if err != nil {
		out.Fail(err, "supervise", false)
		return out
	}
	out.Result = r
	return out
}

// Read reports the daemon of the caller's Herdr session in the repository
// containing cwd, as of now.
func Read(ctx context.Context, cwd string, now time.Time) (Result, error) {
	root, err := fledgedir.Root(ctx, cwd)
	if err != nil {
		return Result{}, err
	}
	session := os.Getenv("HERDR_SESSION")
	st, live, err := supervise.Read(supervise.Dir(root, session))
	r := Result{Session: session, Live: live}
	if err != nil || !live {
		return r, err
	}
	r.Status = &st
	beat, perr := time.Parse(time.RFC3339Nano, st.HeartbeatAt)
	r.Stale = perr != nil || now.Sub(beat) > staleAfter
	return r, nil
}

// Render writes one line about the daemon, then what it watches.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(Result)
	if !ok {
		return nil
	}
	if !r.Live {
		_, err := fmt.Fprintf(w, "Supervisor for session %s: not running.\n", cli.DisplayString(r.Session))
		return err
	}
	st := r.Status
	text := fmt.Sprintf("Supervisor for session %s: running (pid %d, version %s, started %s, heartbeat %s).\n", cli.DisplayString(r.Session), st.PID, cli.DisplayString(st.Version), cli.DisplayString(st.StartedAt), cli.DisplayString(st.HeartbeatAt))
	if r.Stale {
		text += "The heartbeat is stale: the daemon may be hung. Stop it with fledge supervise stop.\n"
	}
	text += fmt.Sprintf("Watching %d: %s\n", len(st.Watched), cli.DisplayString(strings.Join(st.Watched, ", ")))
	_, err := io.WriteString(w, text)
	return err
}
