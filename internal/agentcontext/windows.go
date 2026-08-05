package agentcontext

import "strings"

// claudeWindow returns the context window for a Claude model id. Anthropic's
// standard tier is 200k tokens; the opt-in long-context variants advertise a
// 1M window and are identified by a "1m" marker in the model id. When the id is
// empty the standard window is assumed, which keeps percent populated for the
// common case while never overstating capacity.
func claudeWindow(model string) (int, bool) {
	if strings.Contains(strings.ToLower(model), "1m") {
		return 1_000_000, true
	}
	return 200_000, true
}
