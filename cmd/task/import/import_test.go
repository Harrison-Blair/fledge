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

func TestHelpNamesParentKeysAndBranch(t *testing.T) {
	cmd := New()
	var b bytes.Buffer
	cmd.SetOut(&b)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := strings.Join(strings.Fields(b.String()), " ")
	for _, want := range []string{"base_branch, and branch keys as in task create", "The [parent] also takes shape (stack or feature), branch, base_branch, and delivery_mode"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q: %s", want, b.String())
		}
	}
}
