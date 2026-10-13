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
	for _, want := range []string{"auto delivers\nexactly as pr does, in one command", "local, pr, or auto"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("help lacks %q: %s", want, b.String())
		}
	}
}
