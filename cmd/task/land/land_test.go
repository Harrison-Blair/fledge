package land

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
	for _, want := range []string{"auto does the same\nsteps and merges in one command, with no review stop", "local, pr, or auto", "pr lands in two steps",
		"stops with the pull request open for the owner's\nreview", "task land\n--merge runs every guard", "--merge applies only to pr mode", "--merge "} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("help lacks %q: %s", want, b.String())
		}
	}
}
