package main

import (
	"regexp"
	"testing"
)

// The version number itself is never restated here: the VERSION file is the
// only place it lives, so these tests assert its shape, not its value.

func TestEmbeddedVersionIsANormalizedSemverCore(t *testing.T) {
	t.Parallel()

	version, err := normalizeVersion(embeddedVersion)
	if err != nil {
		t.Fatalf("normalizeVersion(VERSION) error = %v", err)
	}
	// The pattern rejects surrounding whitespace and a leading "v" as well.
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(version) {
		t.Errorf("version = %q, want major.minor.patch", version)
	}
	if embeddedVersion != version+"\n" {
		t.Errorf("VERSION file = %q, want %q and a single trailing newline", embeddedVersion, version)
	}
}

func TestNormalizeVersionTrimsAndValidates(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		raw  string
		want string // empty means normalizeVersion must reject the input
	}{
		{name: "trailing newline", raw: "1.2.3\n", want: "1.2.3"},
		{name: "surrounding whitespace", raw: " \t1.2.3 \n", want: "1.2.3"},
		{name: "leading v", raw: "v1.2.3"},
		{name: "empty", raw: " \n"},
		{name: "missing patch", raw: "1.2"},
		{name: "not a version", raw: "abc"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeVersion(test.raw)
			if test.want == "" {
				if err == nil {
					t.Fatalf("normalizeVersion(%q) = %q, want an error", test.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeVersion(%q) error = %v", test.raw, err)
			}
			if got != test.want {
				t.Errorf("normalizeVersion(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}
