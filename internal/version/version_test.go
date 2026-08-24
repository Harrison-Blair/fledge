package version

import (
	"os"
	"strings"
	"testing"
)

func TestVersionMatchesVersionFile(t *testing.T) {
	raw, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}

	want := strings.TrimSpace(string(raw))
	if want == "" {
		t.Fatal("VERSION must not be empty")
	}

	if got := Version(); got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}
