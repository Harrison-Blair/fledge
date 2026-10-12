// Package stop implements fledge supervise stop: ending the session's
// supervision daemon.
package stop

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/supervise"
)

// Options locates the repository from Cwd.
type Options struct{ Cwd string }

// Result reports whether a live daemon was stopped.
type Result struct {
	Session string `json:"session"`
	Stopped bool   `json:"stopped"`
}

// Run sends SIGTERM to the live daemon of the caller's Herdr session and
// waits up to 10 seconds for it to exit. No live daemon is success.
func Run(ctx context.Context, o Options) cli.Outcome {
	out := cli.NewOutcome("supervise.stop")
	root, err := fledgedir.Root(ctx, o.Cwd)
	if err != nil {
		out.Fail(err, "supervise", false)
		return out
	}
	session := os.Getenv("HERDR_SESSION")
	dir := supervise.Dir(root, session)
	_, live, err := supervise.Read(dir)
	if err == nil && live {
		err = supervise.Stop(ctx, dir)
	}
	out.Result = Result{Session: session, Stopped: live && err == nil}
	if err != nil {
		out.Fail(err, "supervise", true)
		return out
	}
	if live {
		out.Effects = append(out.Effects, cli.Effect{Action: "stopped", Kind: "supervisor", ID: session})
	}
	return out
}

// Render writes the outcome in one line.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(Result)
	if !ok || o.Error != nil {
		return nil
	}
	state := "was not running"
	if r.Stopped {
		state = "stopped"
	}
	_, err := fmt.Fprintf(w, "Supervisor for session %s %s.\n", cli.DisplayString(r.Session), state)
	return err
}
