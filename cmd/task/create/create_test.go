package create

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpNamesAutoMode(t *testing.T) {
	cmd := New()
	var b bytes.Buffer
	cmd.SetOut(&b)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pr and auto each push, open, check, and merge", "local, pr, auto, or none"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("help lacks %q: %s", want, b.String())
		}
	}
}
