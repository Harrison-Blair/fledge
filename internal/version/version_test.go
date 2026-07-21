package version_test

import (
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/version"
)

func TestGet(t *testing.T) {
	got := version.Get()
	if got == "" || got != strings.TrimSpace(got) {
		t.Fatalf("Get() = %q; want non-empty, whitespace-trimmed", got)
	}
}
