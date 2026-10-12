package agent

import (
	"regexp"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// ValidateName enforces the agent-name rule shared by spawn and adopt.
func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return cli.Invalid("--name must match [a-z][a-z0-9_-]{0,31}")
	}
	return nil
}
