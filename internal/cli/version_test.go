package cli

import (
	"os"
	"regexp"
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

// TestStampWarningTxtarVersionMatchesBinary pins the version hardcoded in the
// cmd/fledge/testdata/stamp_warning.txtar fixture (the "binary is X.Y.Z"
// warning assertion) to binaryVersion, so a release bump that updates VERSION
// and version.go but misses this fixture is caught here instead of on CI.
func TestStampWarningTxtarVersionMatchesBinary(t *testing.T) {
	b, err := os.ReadFile("../../cmd/fledge/testdata/stamp_warning.txtar")
	if err != nil {
		t.Fatalf("read stamp_warning.txtar: %v", err)
	}
	m := regexp.MustCompile(`binary is ([0-9\\.]+)`).FindSubmatch(b)
	if m == nil {
		t.Fatalf("no 'binary is X.Y.Z' version found in stamp_warning.txtar")
	}
	// The txtar assertion escapes the dots (0\.5\.5); strip backslashes.
	got := strings.ReplaceAll(string(m[1]), `\`, "")
	if got != binaryVersion {
		t.Errorf("stamp_warning.txtar pinned version = %q, binaryVersion = %q — bump cmd/fledge/testdata/stamp_warning.txtar", got, binaryVersion)
	}
}
