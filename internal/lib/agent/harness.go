package agent

import (
	"slices"
	"strings"
)

// harnesses lists the documented Herdr harness kinds accepted by --harness.
var harnesses = strings.Fields("pi claude codex gemini cursor devin agy cline omp mastracode opencode copilot kimi kiro droid amp grok hermes kilo qodercli qwen letta maki muse")

// Harnesses returns the documented Herdr harness kinds as a fresh slice.
func Harnesses() []string { return slices.Clone(harnesses) }

// IsHarness reports whether kind is a documented Herdr harness kind.
func IsHarness(kind string) bool { return slices.Contains(harnesses, kind) }

// ValidateHarness rejects a --harness value that is not a documented kind.
func ValidateHarness(kind string) error {
	if !IsHarness(kind) {
		return Invalid("--harness must be a documented Herdr harness kind")
	}
	return nil
}
