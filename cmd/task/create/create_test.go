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

func TestHelpNamesShapeAndBranch(t *testing.T) {
	cmd := New()
	var b bytes.Buffer
	cmd.SetOut(&b)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--shape", "stack or feature", "--branch"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("help lacks %q: %s", want, b.String())
		}
	}
}

func TestHelpNamesFeatureMemberDefaults(t *testing.T) {
	cmd := New()
	var b bytes.Buffer
	cmd.SetOut(&b)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := strings.Join(strings.Fields(b.String()), " ")
	for _, want := range []string{"--shape feature needs --branch", "A subtask of a feature parent gets the delivery mode local and the parent's branch as its base branch", "task_feature_member_mode", "task_feature_member_base"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q: %s", want, b.String())
		}
	}
}
