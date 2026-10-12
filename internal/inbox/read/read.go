// Package read implements fledge inbox read: printing the caller's inbox
// records with a ready-made reply command and marking them read.
package read

import (
	"context"
	"fmt"
	"io"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/termtext"
)

// Options names the records to read; none reads every unread record.
type Options struct{ IDs []string }

// Result is the records read, in reading order, after marking them read.
type Result struct {
	Records []inbox.View `json:"records"`
}

// Run reads records addressed to the caller's live record and sets read_at on
// each one not read before. Every named record is checked before any is
// marked, so a refusal changes nothing.
func Run(ctx context.Context, c libagent.Client, o Options) cli.Outcome {
	out := cli.NewOutcome("inbox.read")
	for _, id := range o.IDs {
		if err := libagent.ValidateID("id", "inbox", id); err != nil {
			out.Fail(err, "validation", false)
			return out
		}
	}
	s, err := identity.Existing(ctx, c.Cwd)
	var caller identity.Record
	if err == nil {
		caller, err = identity.RequireCaller(ctx, s, c)
	}
	if err != nil {
		out.Fail(err, "identity", false)
		return out
	}
	var records []inbox.Record
	if len(o.IDs) == 0 {
		all, err := inbox.ForRecipient(s, caller.ID)
		for _, r := range all {
			if inbox.Unread(r) {
				records = append(records, r)
			}
		}
		if err != nil {
			out.Fail(err, "inbox", false)
			return out
		}
	}
	for _, id := range o.IDs {
		r, err := inbox.CheckRecipient(s, id, caller.ID)
		if err != nil {
			out.Fail(err, "inbox", false)
			return out
		}
		records = append(records, r)
	}
	result := Result{Records: []inbox.View{}}
	for _, r := range records {
		read, err := inbox.MarkRead(s, r.ID)
		if err != nil {
			out.Result = result
			out.Fail(err, "inbox", false)
			return out
		}
		if r.ReadAt == nil {
			out.Effects = append(out.Effects, cli.Effect{Action: "updated", Kind: "inbox", ID: r.ID})
		}
		result.Records = append(result.Records, inbox.NewView(read))
	}
	out.Result = result
	return out
}

// Render prints each record's header, cleaned body, and reply command.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(Result)
	if !ok {
		return nil
	}
	if len(r.Records) == 0 {
		_, err := fmt.Fprintln(w, "No unread inbox messages.")
		return err
	}
	for i, v := range r.Records {
		var b strings.Builder
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "── %s %s from %s", v.Kind, v.ID, termtext.Clean(v.From.String()))
		if v.Task != nil {
			fmt.Fprintf(&b, " · task %s", *v.Task)
		}
		if v.ReplyTo != nil {
			fmt.Fprintf(&b, " · reply to %s", *v.ReplyTo)
		}
		fmt.Fprintf(&b, " · %s", v.CreatedAt)
		b.WriteString("\n" + strings.TrimRight(termtext.Clean(v.Body), "\n") + "\n")
		if hint := Reply(v.Record); hint != "" {
			b.WriteString("Reply: " + hint + "\n")
		}
		if _, err := io.WriteString(w, b.String()); err != nil {
			return err
		}
	}
	return nil
}

// Reply is the command that answers r: by record id to a registered sender,
// by pane to an unregistered one, and none for the supervisor or a sender
// outside Herdr.
func Reply(r inbox.Record) string {
	switch {
	case r.From.IsSupervisor():
		return ""
	case r.From.ID != nil:
		return fmt.Sprintf(`fledge agent message --id %s --reply-to %s --body "..."`, *r.From.ID, r.ID)
	case r.From.Pane != nil:
		return fmt.Sprintf(`fledge agent message --pane %s --reply-to %s --body "..."`, termtext.Clean(*r.From.Pane), r.ID)
	}
	return ""
}
