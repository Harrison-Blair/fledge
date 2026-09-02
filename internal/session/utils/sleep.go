// Package utils holds stateless helpers shared by the session package and its
// subpackages. It carries no domain meaning and imports no sibling.
package utils

import (
	"context"
	"time"
)

// Sleep waits for d, reporting why the context ended when it ends first.
func Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
