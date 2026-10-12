package sockettest

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListenUnderLongTempDir(t *testing.T) {
	long := filepath.Join(t.TempDir(), strings.Repeat("d", 120))
	if err := os.Mkdir(long, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", long)
	var dir string
	t.Run("listen", func(t *testing.T) {
		l, path := Listen(t)
		dir = filepath.Dir(path)
		info, err := os.Stat(dir)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("socket directory %s: %v %v", dir, info, err)
		}
		go func() {
			if c, err := l.Accept(); err == nil {
				c.Close()
			}
		}()
		c, err := net.Dial("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	})
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("socket directory %s remains: %v", dir, err)
	}
}

func TestListenUsesShortTempDir(t *testing.T) {
	base, err := os.MkdirTemp("/tmp", "fst-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	t.Setenv("TMPDIR", base)
	_, path := Listen(t)
	if filepath.Dir(filepath.Dir(path)) != base {
		t.Fatalf("socket %s is not under TMPDIR %s", path, base)
	}
}
