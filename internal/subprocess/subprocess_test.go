package subprocess

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestCommandContextSetsWaitDelay(t *testing.T) {
	cmd := CommandContext(context.Background(), os.Args[0], "-test.run=^$")
	if cmd.WaitDelay != 5*time.Second {
		t.Fatalf("WaitDelay = %v, want 5s", cmd.WaitDelay)
	}
}

func TestCommandContextAllowsSuccessfulCommand(t *testing.T) {
	cmd := CommandContext(context.Background(), os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
