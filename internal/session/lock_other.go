//go:build !linux

package session

import (
	"context"
	"fmt"
)

func acquireProjectLock(context.Context, string) (func() error, error) {
	return nil, fmt.Errorf("project locking is unsupported on this platform")
}
