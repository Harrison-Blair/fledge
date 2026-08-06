//go:build !linux && !windows && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package fswatch

import (
	"fmt"
	"runtime"
)

// open keeps every platform compiling. A host without a native change
// notification backend must fail loudly at the point of use rather than be
// silently given a timer, which is exactly what the event-driven design
// exists to avoid.
func open(dir, _ string) (Watcher, error) {
	return nil, fmt.Errorf("watching %q is unsupported on %s: no native filesystem change notification backend", dir, runtime.GOOS)
}
