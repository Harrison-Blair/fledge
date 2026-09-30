package board

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/charmbracelet/x/ansi"
)

// DisplayText is the sole boundary for dynamic terminal text. Strip established
// escape sequences before filtering residual controls; preserve Unicode and lines.
func DisplayText(text string) string {
	// The ANSI parser accepts byte C1 sequences; normalize Unicode-encoded C1
	// introductions before stripping, without corrupting other UTF-8 text.
	text = strings.NewReplacer("\u009b", "\x1b[", "\u009d", "\x1b]", "\u0090", "\x1bP", "\u009c", "\x1b\\", "\u0098", "\x1bX", "\u009e", "\x1b^", "\u009f", "\x1b_").Replace(text)
	text = ansi.Strip(text)
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || (unicode.Is(unicode.Cf, r) && r != '\u200d' && r != '\u200c') {
			return -1
		}
		return r
	}, text)
}
func singleLine(text string) string      { return strings.ReplaceAll(DisplayText(text), "\n", " ") }
func clip(text string, width int) string { return ansi.Truncate(text, max(0, width), "") }

func formatDetail(ctx context.Context, s *Snapshot, id, worker string, width int) []string {
	if ctx.Err() != nil || s == nil {
		return nil
	}
	r, ok := s.Records[id]
	if !ok {
		return nil
	}
	var b strings.Builder
	field := func(label, value string) { fmt.Fprintf(&b, "%s: %s\n", DisplayText(label), DisplayText(value)) }
	optional := func(label string, value *string) {
		if value != nil {
			field(label, *value)
		}
	}
	field("ID", r.ID)
	field("Title", r.Title)
	field("Parent", libagent.Display(r.Parent))
	field("Stored state", r.Status)
	if r.Status == "completed" {
		field("Review", "awaiting verification")
	}
	field("Owner", libagent.Display(r.Owner))
	field("Worker", worker)
	optional("Cancellation reason", r.CancelReason)
	field("Created", r.CreatedAt)
	optional("Created by", r.CreatedBy)
	optional("Assigned", r.AssignedAt)
	optional("Completed", r.CompletedAt)
	optional("Verified", r.VerifiedAt)
	optional("Cancelled", r.CancelledAt)
	optional("Verifier", r.Verifier)
	optional("Verification note", r.VerificationNote)
	if r.Forced {
		field("Forced verification", "yes")
	}
	if len(r.UnmetAtAssign) > 0 {
		field("Unmet at assignment", strings.Join(r.UnmetAtAssign, ", "))
	}
	b.WriteString("\nDependencies:\n")
	if len(r.After) == 0 {
		b.WriteString("none\n")
	}
	for _, depID := range r.After {
		d, ok := s.Records[depID]
		if !ok {
			field(depID, "unknown / unmet")
			continue
		}
		field("Dependency", depID+" · "+d.Title+" · "+d.Status)
		optional("Cancellation reason", d.CancelReason)
	}
	if r.Delivery != nil {
		field("Delivery", r.Delivery.State())
		field("Delivery pane", r.Delivery.Pane)
		field("Delivery message", r.Delivery.MessageID)
	}
	if r.CompletionNotification != nil {
		field("Notification", r.CompletionNotification.State())
		field("Notification recipient", r.CompletionNotification.Recipient)
		field("Notification pane", libagent.Display(r.CompletionNotification.Pane))
		field("Notification message", r.CompletionNotification.MessageID)
	}
	b.WriteString("\nBrief:\n")
	b.WriteString(DisplayText(r.Brief))
	if ctx.Err() != nil {
		return nil
	}
	b.WriteString("\n\nResult:\n")
	b.WriteString(DisplayText(libagent.Display(r.Result)))
	if ctx.Err() != nil {
		return nil
	}
	// This entire operation runs as a command, never in Update or View.
	return strings.Split(ansi.Hardwrap(b.String(), max(1, width), true), "\n")
}
