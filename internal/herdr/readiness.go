package herdr

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	readinessInterval      = 25 * time.Millisecond
	readinessTimeout       = 5 * time.Second
	readinessStableSamples = 2
)

// ReadinessReason is the fixed set of reasons a pane readiness wait can fail
// on its own, as opposed to failing through cancellation or a Herder error.
type ReadinessReason string

const (
	// ReadinessTimeout means the pane never showed two consecutive ready
	// samples before the readiness budget or the caller's deadline ran out.
	ReadinessTimeout ReadinessReason = "timeout"
	// ReadinessUnsupported means this platform cannot observe pane readiness,
	// so the gate fails closed rather than typing into an unready shell.
	ReadinessUnsupported ReadinessReason = "unsupported"
)

var errReadinessUnsupported = errors.New("pane readiness observation is unsupported on this platform")

// ReadinessError reports that a pane did not reach an interactive shell
// prompt. It carries only the pane ID, the reason, and the sampling effort.
type ReadinessError struct {
	PaneID  string
	Reason  ReadinessReason
	Samples int
	Elapsed time.Duration
}

func (e *ReadinessError) Error() string {
	return fmt.Sprintf("herdr agent start: pane %s not ready: %s after %d samples in %s",
		renderArg(e.PaneID), e.Reason, e.Samples, e.Elapsed.Round(time.Millisecond))
}

// readiness holds the pane readiness gate's timing and its observation seam.
// Tests replace the fields in-package; WithSession copies them by value. A
// nil observe means the platform observer, which keeps a fresh Client free of
// func values so callers may still compare clients with reflect.DeepEqual.
type readiness struct {
	interval time.Duration
	timeout  time.Duration
	observe  func(ProcessInfo) (bool, error)
}

func defaultReadiness() readiness {
	return readiness{interval: readinessInterval, timeout: readinessTimeout}
}

func (r readiness) observation() func(ProcessInfo) (bool, error) {
	if r.observe == nil {
		return observePaneReady
	}
	return r.observe
}

// awaitPaneReady polls pane process info until two consecutive samples show
// an interactive shell owning the terminal, or until the smaller of the
// readiness budget and the caller's deadline expires. Caller cancellation and
// Herder errors from process-info are returned as they are; every other
// failure is a ReadinessError.
func (c *Client) awaitPaneReady(ctx context.Context, paneID string) error {
	start := time.Now()
	pollCtx, cancel := context.WithTimeout(ctx, c.readiness.timeout)
	defer cancel()

	observe := c.readiness.observation()
	samples, stable := 0, 0
	fail := func(reason ReadinessReason) error {
		return &ReadinessError{PaneID: paneID, Reason: reason, Samples: samples, Elapsed: time.Since(start)}
	}
	for {
		info, err := c.ProcessInfo(pollCtx, paneID)
		if err != nil {
			if cause := ctx.Err(); cause != nil {
				return readinessCancelled(paneID, cause)
			}
			if pollCtx.Err() != nil {
				return fail(ReadinessTimeout)
			}
			return err
		}
		samples++
		ready, err := observe(info)
		if errors.Is(err, errReadinessUnsupported) {
			return fail(ReadinessUnsupported)
		}
		if ready && err == nil {
			stable++
		} else {
			stable = 0
		}
		if stable >= readinessStableSamples {
			return nil
		}

		wait := time.NewTimer(c.readiness.interval)
		select {
		case <-pollCtx.Done():
			wait.Stop()
			if cause := ctx.Err(); cause != nil {
				return readinessCancelled(paneID, cause)
			}
			return fail(ReadinessTimeout)
		case <-wait.C:
		}
	}
}

func readinessCancelled(paneID string, cause error) error {
	return withContextCause(fmt.Errorf("herdr agent start: pane %s readiness: %w", renderArg(paneID), cause), cause)
}
