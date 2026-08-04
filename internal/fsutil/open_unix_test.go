//go:build !windows

package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenNoFollowCannotTruncateSymlinkTarget(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	link := filepath.Join(directory, "link")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	file, err := OpenNoFollow(link, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err == nil {
		_ = file.Close()
		t.Fatal("OpenNoFollow unexpectedly opened a symlink")
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "unchanged" {
		t.Fatalf("symlink target was truncated: %q", contents)
	}
}
