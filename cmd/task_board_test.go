package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestTaskBoardHelpAndNonTTY(t *testing.T) {
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("q"))
	root.SetArgs([]string{"task", "board", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "read-only") || !strings.Contains(out.String(), "history") {
		t.Fatalf("help %s", out.String())
	}
	out.Reset()
	root = NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("q"))
	root.SetArgs([]string{"task", "board"})
	err := root.Execute()
	exit, ok := err.(interface{ ExitCode() int })
	if !ok || exit.ExitCode() != 2 {
		t.Fatalf("error %v", err)
	}
	if !strings.Contains(out.String(), "terminal") {
		t.Fatal(out.String())
	}
}
