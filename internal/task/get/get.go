// Package get implements task get: one task's full record, the progress of
// its subtasks, and the state of its prerequisites.
package get

import (
	"context"
	"fmt"
	"io"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/usage"
)

type Options struct{ ID string }

// Result is the task with the progress of its direct children, null when it
// has none.
type Result struct {
	task.Record
	Progress     *task.Progress `json:"progress"`
	Dependencies []Dependency   `json:"dependencies"`
}

// Dependency is one prerequisite in declaration order with its current
// state. A verified or cancelled prerequisite is satisfied.
type Dependency struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Status       string  `json:"status"`
	CancelReason *string `json:"cancel_reason"`
	Satisfied    bool    `json:"satisfied"`
}

// Run reads one task from the store without contacting Herdr.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.get", Status: "success", Effects: []libagent.Effect{}}
	if err := task.ValidateID(o.ID); err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := task.Existing(ctx, c.Cwd)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	r, err := task.Get(s, o.ID)
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	rs, err := task.List(s)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	byID := task.Index(rs)
	deps := []Dependency{}
	for _, id := range r.After {
		dep := byID[id]
		deps = append(deps, Dependency{ID: id, Title: dep.Title, Status: dep.Status, CancelReason: dep.CancelReason, Satisfied: task.Satisfied(dep.Status)})
	}
	out.Result = Result{Record: r, Progress: task.ChildProgress(r.ID, rs), Dependencies: deps}
	return out
}

// Render writes the task as labelled lines, omitting steps not yet reached,
// followed by its usage snapshots and indented texts.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	var b strings.Builder
	owner := "-"
	if r.Owner != nil {
		owner = *r.Owner
	}
	creator := "an unregistered caller"
	if r.CreatedBy != nil {
		creator = *r.CreatedBy
	}
	fmt.Fprintf(&b, "id: %s\ntitle: %s\nstatus: %s\nowner: %s\n", r.ID, r.Title, r.Status, owner)
	if r.Parent != nil {
		fmt.Fprintf(&b, "parent: %s\n", *r.Parent)
	}
	if r.Progress != nil {
		fmt.Fprintf(&b, "subtasks: %s\n", r.Progress)
	}
	if len(r.Dependencies) > 0 {
		deps := []string{}
		for _, d := range r.Dependencies {
			state := d.Status
			switch {
			case d.Status == task.Cancelled && d.CancelReason != nil:
				state += ": " + *d.CancelReason
			case !d.Satisfied:
				state += ", waiting"
			}
			deps = append(deps, fmt.Sprintf("%s (%s)", d.ID, state))
		}
		fmt.Fprintf(&b, "after: %s\n", strings.Join(deps, ", "))
	}
	fmt.Fprintf(&b, "created: %s by %s\n", r.CreatedAt, creator)
	if r.AssignedAt != nil {
		forced := ""
		if r.UnmetAtAssign != nil {
			forced = fmt.Sprintf(" (forced; unmet then: %s)", strings.Join(r.UnmetAtAssign, ", "))
		}
		fmt.Fprintf(&b, "assigned: %s%s\n", *r.AssignedAt, forced)
	}
	if d := r.Delivery; d != nil {
		fmt.Fprintf(&b, "delivery: message %s to %s, %s\n", d.MessageID, d.Pane, d.State())
	}
	if r.CompletedAt != nil {
		fmt.Fprintf(&b, "completed: %s\n", *r.CompletedAt)
	}
	if n := r.CompletionNotification; n != nil {
		target := n.Recipient
		if n.Pane != nil {
			target += " in " + *n.Pane
		}
		fmt.Fprintf(&b, "completion notification: message %s to %s, %s\n", n.MessageID, target, n.State())
	}
	if r.VerifiedAt != nil {
		verifier, forced := "an unregistered caller", ""
		if r.Verifier != nil {
			verifier = *r.Verifier
		}
		if r.Forced {
			forced = " (forced)"
		}
		fmt.Fprintf(&b, "verified: %s by %s%s\n", *r.VerifiedAt, verifier, forced)
	}
	if r.CancelledAt != nil {
		reason := ""
		if r.CancelReason != nil {
			reason = " (" + *r.CancelReason + ")"
		}
		fmt.Fprintf(&b, "cancelled: %s%s\n", *r.CancelledAt, reason)
	}
	if u := r.Usage; u != nil {
		b.WriteString("usage:\n")
		snapshot(&b, "worker", u.Worker)
		snapshot(&b, "verifier", u.Verifier)
	}
	text(&b, "brief", &r.Brief)
	text(&b, "result", r.Result)
	text(&b, "verification note", r.VerificationNote)
	_, err := io.WriteString(w, b.String())
	return err
}

func text(b *strings.Builder, label string, s *string) {
	if s == nil || *s == "" {
		return
	}
	fmt.Fprintf(b, "%s:\n", label)
	for _, line := range strings.Split(strings.TrimRight(*s, "\n"), "\n") {
		fmt.Fprintf(b, "  %s\n", line)
	}
}

// snapshot writes one usage line: elapsed time, then counts, cost, and basis,
// or only the basis when usage is unavailable. The reason follows the basis.
func snapshot(b *strings.Builder, label string, u *task.UsageSnapshot) {
	if u == nil {
		return
	}
	basis := u.Basis
	if u.Reason != nil {
		basis += " (" + *u.Reason + ")"
	}
	elapsed := duration(u.ElapsedSeconds)
	if u.Basis == usage.Unavailable {
		fmt.Fprintf(b, "  %s: %s, %s\n", label, elapsed, basis)
		return
	}
	cost := "-"
	if c := u.Cost; c != nil {
		cost = fmt.Sprintf("%.2f %s (est)", c.Amount, c.Currency)
		if c.Currency == "USD" || c.Currency == "" {
			cost = fmt.Sprintf("$%.2f (est)", c.Amount)
		}
	}
	t := u.Tokens
	fmt.Fprintf(b, "  %s: %s, %d turns, in %s out %s cache-r %s cache-w %s, cost %s, %s\n",
		label, elapsed, u.Turns, count(t.Input), count(t.Output), count(t.CacheRead), count(t.CacheWrite), cost, basis)
}

// duration renders seconds as 1h02m, 5m12s, or 40s.
func duration(s int64) string {
	switch {
	case s >= 3600:
		return fmt.Sprintf("%dh%02dm", s/3600, s%3600/60)
	case s >= 60:
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	}
	return fmt.Sprintf("%ds", s)
}

// count renders a token count with a k or M suffix: 12, 1.2k, 96k, 410k, 2.3M.
func count(n int64) string {
	f := float64(n)
	switch {
	case n < 1000:
		return fmt.Sprint(n)
	case n < 99_950:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", f/1e3), ".0") + "k"
	case n < 999_500:
		return fmt.Sprintf("%.0fk", f/1e3)
	case n < 99_950_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", f/1e6), ".0") + "M"
	}
	return fmt.Sprintf("%.0fM", f/1e6)
}
