package cli

import (
	"os"
	"strings"
	"testing"
)

// TestBinaryVersionMatchesVersionFile pins the compiled-in default version to
// the repo-root VERSION file so a release bump can't leave the binary stale.
func TestBinaryVersionMatchesVersionFile(t *testing.T) {
	b, err := os.ReadFile("../../VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	want := strings.TrimSpace(string(b))
	if binaryVersion != want {
		t.Errorf("binaryVersion = %q, VERSION file = %q — bump internal/cli/version.go", binaryVersion, want)
	}
}
