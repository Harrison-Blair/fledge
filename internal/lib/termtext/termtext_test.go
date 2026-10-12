package termtext

import (
	"strings"
	"testing"
	"unicode"
)

func TestClean(t *testing.T) {
	text := "hé世界\n\tgood\r\b\x00\x1b[31mred\x1b[0m\x1b]52;c;SECRET\a\x1bPSECRET\x1b\\\u009b31mC1‮evil"
	got := Clean(text)
	if strings.Contains(got, "SECRET") || strings.Contains(got, "31m") || strings.Contains(got, "‮") {
		t.Fatalf("unsafe: %q", got)
	}
	if !strings.Contains(got, "hé世界\n") || !strings.Contains(got, "red") {
		t.Fatalf("lost text: %q", got)
	}
	for _, r := range got {
		if unicode.IsControl(r) && r != '\n' {
			t.Fatalf("control %U", r)
		}
	}
}

func TestCleanUnicodeStringControls(t *testing.T) {
	for _, intro := range []string{"\u0090", "\u0098", "\u009d", "\u009e", "\u009f"} {
		if got := Clean("before" + intro + "SECRET\u009cafter"); got != "beforeafter" {
			t.Errorf("intro %q: %q", intro, got)
		}
	}
}

func TestCleanIncompleteSequenceAndJoiners(t *testing.T) {
	if got := Clean("a‍b‌c\x1b[31"); strings.ContainsRune(got, '\x1b') || !strings.HasPrefix(got, "a‍b‌c") {
		t.Fatalf("got %q", got)
	}
}
