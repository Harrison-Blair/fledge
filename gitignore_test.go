package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestTrackedFilesAreNotIgnored checks that the allowlist .gitignore allows
// every tracked file, so a new file at the same path is not silently ignored.
func TestTrackedFilesAreNotIgnored(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		t.Skip("not inside a git work tree")
	}
	out, err := exec.Command("git", "ls-files", "--cached", "--ignored", "--exclude-standard").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	if ignored := strings.TrimSpace(string(out)); ignored != "" {
		t.Errorf(".gitignore ignores tracked files; add the narrowest allow rule for each:\n%s", ignored)
	}
}
