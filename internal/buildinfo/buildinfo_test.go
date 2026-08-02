package buildinfo

import (
	"regexp"
	"testing"
)

const (
	semverNumericIdentifier    = `(0|[1-9][0-9]*)`
	semverPrereleaseIdentifier = `(` + semverNumericIdentifier + `|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)`
	semverPattern              = `^v` + semverNumericIdentifier + `\.` + semverNumericIdentifier + `\.` + semverNumericIdentifier +
		`(-` + semverPrereleaseIdentifier + `(\.` + semverPrereleaseIdentifier + `)*)?` +
		`(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`
)

var validVersion = regexp.MustCompile(semverPattern)

func TestEmbeddedVersion(t *testing.T) {
	got := Version()
	if !validVersion.MatchString(got) {
		t.Fatalf("version %q is not canonical semantic version prefixed with v", got)
	}
	info := Current()
	if info.Version != got || info.GoVersion == "" {
		t.Fatalf("info = %#v", info)
	}
}

func TestVersionValidation(t *testing.T) {
	for _, test := range []struct {
		version string
		valid   bool
	}{
		{version: "v0.0.0", valid: true},
		{version: "v1.2.3-rc.1+build.5", valid: true},
		{version: ""},
		{version: "1.2.3"},
		{version: "v1.2"},
		{version: "v01.2.3"},
		{version: "v1.2.3-01"},
	} {
		if got := validVersion.MatchString(test.version); got != test.valid {
			t.Errorf("validVersion.MatchString(%q) = %t, want %t", test.version, got, test.valid)
		}
	}
}
