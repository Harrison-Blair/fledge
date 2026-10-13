package importcmd

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
	if want := "(local, pr, auto, or none)"; !strings.Contains(b.String(), want) {
		t.Errorf("help lacks %q: %s", want, b.String())
	}
}
