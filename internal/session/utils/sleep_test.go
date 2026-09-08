package utils_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"fledge/internal/session/utils"
)

func TestSleepReturnsNilAfterDuration(t *testing.T) {
	t.Parallel()

	if err := utils.Sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("Sleep after elapse: got %v, want nil", err)
	}
}

func TestSleepReturnsContextErrWhenCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := utils.Sleep(ctx, time.Hour)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Sleep with cancelled context: got %v, want context.Canceled", err)
	}
	if elapsed > time.Second {
		t.Fatalf("Sleep did not return promptly on cancellation: took %v", elapsed)
	}
}
