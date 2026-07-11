package cli

import (
	"strings"
	"testing"
)

// TestPromptYesNo: only an explicit yes answers true; anything else — "n",
// empty line, EOF — answers false.
func TestPromptYesNo(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"n\n", false},
		{"no\n", false},
		{"\n", false},
		{"", false}, // EOF
	} {
		var out strings.Builder
		got := promptYesNo(strings.NewReader(tc.input), &out, "overwrite? [y/N] ")
		if got != tc.want {
			t.Errorf("promptYesNo(%q) = %v, want %v", tc.input, got, tc.want)
		}
		if out.String() != "overwrite? [y/N] " {
			t.Errorf("prompt not written: %q", out.String())
		}
	}
}
