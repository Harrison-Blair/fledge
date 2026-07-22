package version_test

import (
	"regexp"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/version"
)

// semver matches a strict MAJOR.MINOR.PATCH: numeric components with no leading
// zeros (a bare "0" is allowed). It is anchored, so any surrounding whitespace
// also fails the match.
var semver = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func TestGet(t *testing.T) {
	got := version.Get()
	if !semver.MatchString(got) {
		t.Fatalf("Get() = %q; want strict MAJOR.MINOR.PATCH", got)
	}
}
