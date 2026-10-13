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
	for _, want := range []string{"pr pushes, opens, and checks a pull request and\nstops for review until task land --merge", "auto pushes, opens, checks, and\nmerges a pull request in one command", "local, pr, auto, or none"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("help lacks %q: %s", want, b.String())
		}
	}
}
