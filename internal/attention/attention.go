// Package attention implements fledge attention: a read-only summary of what
// needs the caller now.
package attention

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	libinbox "github.com/Harrison-Blair/fledge/internal/inbox"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/supervise"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/termtext"
)

// Groups is the order workers are shown in. quiet appears only while the
// supervision daemon is live.
var Groups = []string{"blocked", "quiet", "working", "idle", "done", "unknown"}

// Options bounds inbox previews to Width; Now dates their ages.
type Options struct {
	Width int
	Now   time.Time
}

// Worker is one live worker the caller spawned and its group.
type Worker struct {
	ID    string  `json:"id"`
	Name  *string `json:"name"`
	Pane  *string `json:"pane"`
	Group string  `json:"group"`
}

// Verify is a completed task the caller created and has not verified.
type Verify struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Owner *string `json:"owner"`
}

// Supervisor is the session's supervision daemon as its lock reports it.
type Supervisor struct {
	Live    bool   `json:"live"`
	Version string `json:"version"`
}

// Result is what needs the caller: its unread inbox, its workers, tasks to
// verify, and whether the daemon runs.
type Result struct {
	Caller     string       `json:"caller"`
	Inbox      []inbox.View `json:"inbox"`
	Workers    []Worker     `json:"workers"`
	Verify     []Verify     `json:"verify"`
	Supervisor Supervisor   `json:"supervisor"`
	width      int
	now        time.Time
}

// Run builds the summary from one agent.list call and the store. It never
// writes: the caller and its workers are matched by terminal without the
// record updates other lookups make.
func Run(ctx context.Context, c libagent.Client, o Options) cli.Outcome {
	out := cli.NewOutcome("attention")
	s, err := identity.Existing(ctx, c.Cwd)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	agents, err := c.List(ctx)
	if err != nil {
		out.Fail(err, "agent.list", false)
		return out
	}
	records, err := identity.LiveByTerminal(s)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	var caller *identity.Record
	for _, a := range agents {
		if rec, ok := identity.Attributed(records, a); ok && a.PaneID == c.CallerPane && c.CallerPane != "" {
			caller = &rec
		}
	}
	if caller == nil {
		out.Fail(&cli.Error{Code: "caller_unregistered", Message: "the caller has no live Fledge record; register with fledge agent adopt"}, "identity", false)
		return out
	}
	r := Result{Caller: caller.ID, Inbox: []inbox.View{}, Workers: []Worker{}, Verify: []Verify{}, width: o.Width, now: o.Now}
	dir := supervise.DirIn(s.Dir(), os.Getenv("HERDR_SESSION"))
	st, live, err := supervise.Read(dir)
	var quiet map[string]bool
	if err == nil && live {
		r.Supervisor = Supervisor{Live: true, Version: st.Version}
		ep, eerr := supervise.LoadEpisodes(dir)
		quiet, err = ep.Quiet, eerr
	}
	var msgs []inbox.Record
	if err == nil {
		msgs, err = inbox.ForRecipient(s, caller.ID)
	}
	var children []identity.Record
	if err == nil {
		children, err = identity.Children(s, caller.ID)
	}
	var tasks []task.Record
	if err == nil {
		tasks, err = task.List(s)
	}
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	for _, m := range msgs {
		if inbox.Unread(m) {
			r.Inbox = append(r.Inbox, inbox.NewView(m))
		}
	}
	for _, rec := range children {
		if rec.EndedAt != nil {
			continue
		}
		w := Worker{ID: rec.ID, Name: rec.Name, Group: "unknown"}
		for _, a := range agents {
			if live, ok := identity.Attributed(records, a); ok && live.ID == rec.ID {
				w.Pane, w.Group = &a.PaneID, a.AgentStatus
			}
		}
		if quiet[rec.ID] {
			w.Group = "quiet"
		}
		if !slices.Contains(Groups, w.Group) {
			w.Group = "unknown"
		}
		r.Workers = append(r.Workers, w)
	}
	slices.SortStableFunc(r.Workers, func(a, b Worker) int { return slices.Index(Groups, a.Group) - slices.Index(Groups, b.Group) })
	for _, t := range tasks {
		if t.Status == task.Completed && t.CreatedBy != nil && *t.CreatedBy == caller.ID {
			r.Verify = append(r.Verify, Verify{ID: t.ID, Title: t.Title, Owner: t.Owner})
		}
	}
	out.Result = r
	return out
}

// Render writes the inbox, the workers by group, the tasks to verify, and
// the daemon's state.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(Result)
	if !ok {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Inbox: %d unread\n", len(r.Inbox))
	if len(r.Inbox) > 0 {
		b.WriteString(libinbox.Table(r.Inbox, r.width, r.now) + "\n")
	}
	b.WriteString("Workers:\n")
	if len(r.Workers) == 0 {
		b.WriteString("  none\n")
	}
	for _, wk := range r.Workers {
		name := wk.ID
		if wk.Name != nil {
			name = fmt.Sprintf("%s (%s)", termtext.Clean(*wk.Name), wk.ID)
		}
		fmt.Fprintf(&b, "  %s: %s in %s\n", wk.Group, name, cli.Display(wk.Pane))
	}
	b.WriteString("Tasks to verify:\n")
	if len(r.Verify) == 0 {
		b.WriteString("  none\n")
	}
	for _, t := range r.Verify {
		fmt.Fprintf(&b, "  %s %s (owner %s)\n", t.ID, termtext.Clean(t.Title), cli.Display(t.Owner))
	}
	if r.Supervisor.Live {
		fmt.Fprintf(&b, "Supervisor: running, version %s\n", cli.DisplayString(r.Supervisor.Version))
	} else {
		b.WriteString("Supervisor: not running\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
