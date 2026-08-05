package agentcontext

import (
	"fmt"
	"strings"
)

// Render returns a deterministic, human-readable table of the report. An
// available agent shows its window, used tokens, and percent (or the used total
// when the window is unknown); an unknown agent shows its stable reason.
func Render(report Report) string {
	if len(report.Agents) == 0 {
		return "No live agents.\n"
	}
	var builder strings.Builder
	for _, agent := range report.Agents {
		fmt.Fprintf(&builder, "%s (%s)", agent.Name, agent.Harness)
		switch {
		case agent.Status == StatusAvailable && agent.Percent != nil && agent.Window != nil && agent.Used != nil:
			fmt.Fprintf(&builder, ": %d/%d tokens (%.2f%%)", *agent.Used, *agent.Window, *agent.Percent)
		case agent.Status == StatusAvailable && agent.Used != nil:
			fmt.Fprintf(&builder, ": %d tokens used (context window unknown)", *agent.Used)
		case agent.Reason != nil:
			fmt.Fprintf(&builder, ": unknown (%s)", *agent.Reason)
		default:
			builder.WriteString(": unknown")
		}
		builder.WriteByte('\n')
	}
	return builder.String()
}
