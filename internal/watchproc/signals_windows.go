//go:build windows

package watchproc

import (
	"context"
	"os"
	"os/signal"
)

func dispatcherContext(parent context.Context) (context.Context, func()) {
	return signal.NotifyContext(parent, os.Interrupt)
}
