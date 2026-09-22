// Package read implements agent read: a terminal snapshot of a live agent's pane.
package read

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

// Options selects one live agent and the snapshot to capture. Source uses the
// CLI spelling; Lines applies only when LinesSet.
type Options struct {
	Name, Pane, Source string
	Lines              int64
	LinesSet           bool
}

// Result is the agent row plus the captured snapshot. Lines counts returned rows.
type Result struct {
	libagent.AgentRow
	Source    string `json:"source"`
	Lines     int    `json:"lines"`
	Text      string `json:"text"`
	Revision  uint64 `json:"revision"`
	Truncated bool   `json:"truncated"`
}

// wireSources maps CLI source spellings to Herdr's; the hyphenated spelling is
// rejected on the wire.
var wireSources = map[string]string{"visible": "visible", "recent": "recent", "recent-unwrapped": "recent_unwrapped", "detection": "detection"}

// Run resolves the agent, then reads its pane without focusing it or marking output seen.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.read", Status: "success", Effects: []libagent.Effect{}}
	target, err := libagent.ResolveTarget(o.Name, o.Pane)
	source, known := wireSources[o.Source]
	if err == nil && !known {
		err = libagent.Invalid("--source must be visible, recent, recent-unwrapped, or detection")
	}
	if err == nil && o.LinesSet && (o.Lines < 0 || o.Lines > math.MaxUint32) {
		err = libagent.Invalid("--lines must be between 0 and %d", uint32(math.MaxUint32))
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	a, err := c.Get(ctx, target)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	var lines *uint32
	if o.LinesSet {
		n := uint32(o.Lines)
		lines = &n
	}
	r, err := c.Read(ctx, a.PaneID, source, lines)
	if err == nil && r.PaneID != a.PaneID {
		err = libagent.Protocol("agent.read returned a different pane")
	}
	if err != nil {
		out.Fail(err, "agent.read", false)
		return out
	}
	out.Result = Result{AgentRow: libagent.NewAgentRow(a.Pane), Source: o.Source, Lines: rows(r.Text), Text: r.Text, Revision: *r.Revision, Truncated: *r.Truncated}
	return out
}

func rows(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(text, "\n"), "\n") + 1
}

// Render writes a labeled snapshot followed by its text, ending in a newline.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	who := r.PaneID
	if r.Name != nil && *r.Name != "" {
		who = r.Name
	}
	truncated := "no"
	if r.Truncated {
		truncated = "yes"
	}
	text := r.Text
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	_, err := fmt.Fprintf(w, "Terminal snapshot of %s (%s, %d rows, truncated: %s)\n%s", display(who), r.Source, r.Lines, truncated, text)
	return err
}
func display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
