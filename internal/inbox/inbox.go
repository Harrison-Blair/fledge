// Package inbox implements fledge inbox: listing the caller's inbox records.
package inbox

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	libinbox "github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/termtext"
)

// Options selects unread records unless All is set. Width bounds each body
// preview line; Now is the time ages are measured from.
type Options struct {
	All   bool
	Width int
	Now   time.Time
}

// Result lists the caller's records, unread first, each group oldest first.
type Result struct {
	Caller  string          `json:"caller"`
	Records []libinbox.View `json:"records"`
	width   int
	now     time.Time
}

// Run lists the inbox of the caller's live record. It never writes.
func Run(ctx context.Context, c libagent.Client, o Options) cli.Outcome {
	out := cli.NewOutcome("inbox.list")
	s, err := identity.Existing(ctx, c.Cwd)
	var caller identity.Record
	if err == nil {
		caller, err = identity.RequireCaller(ctx, s, c)
	}
	if err != nil {
		out.Fail(err, "identity", false)
		return out
	}
	records, err := libinbox.ForRecipient(s, caller.ID)
	if err != nil {
		out.Fail(err, "inbox", false)
		return out
	}
	result := Result{Caller: caller.ID, Records: []libinbox.View{}, width: o.Width, now: o.Now}
	for _, r := range records {
		if o.All || libinbox.Unread(r) {
			result.Records = append(result.Records, libinbox.NewView(r))
		}
	}
	out.Result = result
	return out
}

// Render writes one table row per record with its first body line beneath,
// cleaned and cut to the width.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(Result)
	if !ok {
		return nil
	}
	if len(r.Records) == 0 {
		_, err := fmt.Fprintln(w, "No unread inbox messages.")
		return err
	}
	if _, err := fmt.Fprintln(w, Table(r.Records, r.width, r.now)); err != nil {
		return err
	}
	return nil
}

// Table renders records as the inbox listing without a trailing newline;
// fledge attention shows unread records the same way. Columns are padded to
// their widest cell, so preview lines between rows keep them aligned.
func Table(records []libinbox.View, width int, now time.Time) string {
	rows := [][]string{{"ID", "STATE", "KIND", "FROM", "TASK", "AGE"}}
	for _, r := range records {
		rows = append(rows, []string{r.ID, r.State, r.Kind, termtext.Clean(r.From.String()), cli.Display(r.Task), Age(r.CreatedAt, now)})
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], ansi.StringWidth(cell))
		}
	}
	var lines []string
	for i, row := range rows {
		var line strings.Builder
		for j, cell := range row {
			line.WriteString(cell)
			if j < len(row)-1 {
				line.WriteString(strings.Repeat(" ", widths[j]-ansi.StringWidth(cell)+2))
			}
		}
		lines = append(lines, line.String())
		if i > 0 {
			lines = append(lines, Preview(records[i-1].Body, width))
		}
	}
	return strings.Join(lines, "\n")
}

// Preview is body's first nonblank line, cleaned, indented two spaces, and cut
// to width display cells when width is positive.
func Preview(body string, width int) string {
	line := ""
	for _, l := range strings.Split(termtext.Clean(body), "\n") {
		if strings.TrimSpace(l) != "" {
			line = strings.TrimSpace(l)
			break
		}
	}
	line = "  " + line
	if width > 0 {
		line = ansi.Truncate(line, width, "…")
	}
	return line
}

// Age is the time from created to now in its largest whole unit, or - when
// created does not parse.
func Age(created string, now time.Time) string {
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
