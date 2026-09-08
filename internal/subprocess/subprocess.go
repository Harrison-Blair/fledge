// Package subprocess constructs commands with Fledge's shared cleanup policy.
package subprocess

import (
	"context"
	"os/exec"
	"time"
)

const waitDelay = 5 * time.Second

// CommandContext creates a command that gives inherited pipes a bounded time
// to close after the process exits or its context is cancelled.
func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.WaitDelay = waitDelay
	return cmd
}
