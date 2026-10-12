package landing

import (
	"context"
	"slices"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/repoconfig"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// ValidMode checks a delivery mode named by flag.
func ValidMode(flag, mode string) error {
	if !slices.Contains(task.DeliveryModes, mode) {
		return cli.Invalid("%s must be one of %s", flag, strings.Join(task.DeliveryModes, ", "))
	}
	return nil
}

// DefaultMode is the delivery mode of a task created without one: the
// delivery_mode of the repository containing cwd, else none.
func DefaultMode(ctx context.Context, cwd string) (string, error) {
	root, err := fledgedir.Root(ctx, cwd)
	if err != nil {
		return "", err
	}
	c, err := repoconfig.Read(ctx, root)
	if err != nil || c.DeliveryMode == "" {
		return task.ModeNone, err
	}
	return c.DeliveryMode, nil
}
