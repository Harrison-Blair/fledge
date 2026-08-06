package watchproc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Harrison-Blair/fledge/internal/fswatch"
	"github.com/Harrison-Blair/fledge/internal/statedir"
)

// WaitReady waits on filesystem notifications from the dispatcher state
// directory. It never polls for a marker.
func WaitReady(ctx context.Context, root, session string) error {
	path := filepath.Join(statedir.TempSession(root, session), readyFilename)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	changes, err := fswatch.File(path)
	if err != nil {
		return err
	}
	defer changes.Close()
	// The marker can appear between the stat above and the watch: re-stat once
	// the watch is armed so that race resolves as ready rather than as a wait.
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for dispatcher readiness: %w", ctx.Err())
		case err := <-changes.Errors():
			return fmt.Errorf("wait for dispatcher readiness: %w", err)
		case <-changes.Events():
			if _, err := os.Stat(path); err == nil {
				return nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
}
