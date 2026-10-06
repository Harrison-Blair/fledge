package agent

import (
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/harness"
)

// ValidateHarness rejects a --harness value that is not a documented kind.
func ValidateHarness(kind string) error {
	if !harness.IsKind(kind) {
		return cli.Invalid("--harness must be a documented Herdr harness kind")
	}
	return nil
}
