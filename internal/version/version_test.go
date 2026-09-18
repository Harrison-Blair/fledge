package version

import (
	"os"
	"strings"
	"testing"
)

func TestVersionMatchesFile(t *testing.T) {
	data, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(data))
	if want == "" {
		t.Fatal("VERSION is empty")
	}
	if got := Version(); got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
	t.Chdir(t.TempDir())
	if got := Version(); got != want {
		t.Fatalf("Version() outside repository = %q, want %q", got, want)
	}
}
