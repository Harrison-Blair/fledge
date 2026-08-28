//go:build !linux

package lock

import (
	"context"
	"fmt"
)

// Acquire reports that the project lock is unavailable on this platform.
func Acquire(context.Context, string) (func() error, error) {
	return nil, fmt.Errorf("project locking is unsupported on this platform")
}
