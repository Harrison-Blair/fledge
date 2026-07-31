package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMessageRunsDoesNotRequireRuntimeStateOrHerdr(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	stateFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(stateFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", stateFile)
	var out, errOut bytes.Buffer
	code := Execute(context.Background(), []string{
		"agent", "message", "runs", "--json", "--herdr-bin", filepath.Join(t.TempDir(), "missing"),
	}, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}

func TestMessageCommandsRequireExactlyOneSource(t *testing.T) {
	cases := [][]string{
		{"agent", "message", "send", "worker"},
		{"agent", "message", "send", "worker", "text", "--file", "-"},
		{"agent", "message", "send", "worker", "--file="},
		{"agent", "message", "reply", "msg_example"},
		{"agent", "message", "reply", "msg_example", "text", "--file", "reply.txt"},
	}
	for _, args := range cases {
		var out, errOut bytes.Buffer
		code := Execute(context.Background(), append(args, "--json"), bytes.NewBufferString("body"), &out, &errOut)
		if code != 2 {
			t.Fatalf("%v: code=%d stdout=%s stderr=%s", args, code, out.String(), errOut.String())
		}
		var envelope errorEnvelope
		if err := json.Unmarshal(errOut.Bytes(), &envelope); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if envelope.Error.Code != "usage_error" {
			t.Fatalf("%v: error=%#v", args, envelope.Error)
		}
	}
}

func TestMessageHistoryRejectsConflictingRunSelectors(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Execute(context.Background(), []string{
		"agent", "message", "history", "worker",
		"--run", "run_example", "--all-runs", "--json",
	}, nil, &out, &errOut)
	if code != 2 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}
