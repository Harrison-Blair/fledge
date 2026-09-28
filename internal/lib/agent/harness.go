package agent

import "github.com/Harrison-Blair/fledge/internal/lib/harness"

// Harnesses returns the documented Herdr harness kinds as a fresh slice.
func Harnesses() []string { return harness.Kinds() }

// IsHarness reports whether kind is a documented Herdr harness kind.
func IsHarness(kind string) bool { return harness.IsKind(kind) }

// ValidateHarness rejects a --harness value that is not a documented kind.
func ValidateHarness(kind string) error {
	if !IsHarness(kind) {
		return Invalid("--harness must be a documented Herdr harness kind")
	}
	return nil
}
