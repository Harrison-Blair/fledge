// Package sockettest starts Unix socket listeners for tests whose temporary
// directory can be too long for a socket path. It imports only the standard
// library, so any package's tests can use it. Production packages must not
// import it.
package sockettest

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// maxPath is the longest socket path that every supported platform accepts:
// macOS limits sun_path to 104 bytes, including the terminating NUL.
const maxPath = 103

// Listen starts a Unix socket listener in a new private directory and returns
// it with its path. The directory is under the temporary directory when the
// path fits, and under /tmp otherwise. Cleanup closes the listener and removes
// the directory.
func Listen(t testing.TB) (net.Listener, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "fs-")
	if err == nil && len(filepath.Join(dir, "s")) > maxPath {
		os.Remove(dir)
		dir, err = os.MkdirTemp("/tmp", "fs-")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l, path
}
