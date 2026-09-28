package harness

import (
	"slices"
	"strings"
)

// kinds lists the documented Herdr harness kinds.
var kinds = strings.Fields("pi claude codex gemini cursor devin agy cline omp mastracode opencode copilot kimi kiro droid amp grok hermes kilo qodercli qwen letta maki muse")

// Kinds returns the documented Herdr harness kinds as a fresh slice.
func Kinds() []string { return slices.Clone(kinds) }

// IsKind reports whether kind is a documented Herdr harness kind.
func IsKind(kind string) bool { return slices.Contains(kinds, kind) }
