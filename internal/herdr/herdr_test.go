package herdr

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fakeHerdr(t *testing.T, script string) {
	t.Helper()
	binDir := t.TempDir()
	path := filepath.Join(binDir, "herdr")
	contents := "#!/bin/sh\nset -eu\n" + script
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestList(t *testing.T) {
	fakeHerdr(t, `
test "$#" -eq 3
test "$1" = session
test "$2" = list
test "$3" = --json
printf '%s' "$HERDR_FAKE_OUTPUT"
`)
	t.Setenv("HERDR_FAKE_OUTPUT", `{"sessions":[{"name":"alpha","running":true,"default":false,"socket_path":"/tmp/alpha.sock","future_field":"accepted"},{"name":"old","running":false,"default":false,"socket_path":"/tmp/old.sock"}],"future_field":"accepted"}`)

	got, err := New(nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []Session{
		{Name: "alpha", Running: true, SocketPath: "/tmp/alpha.sock"},
		{Name: "old", SocketPath: "/tmp/old.sock"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %#v, want %#v", got, want)
	}
}

func TestListRejectsInvalidOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "invalid JSON", output: `{`, want: "decode output"},
		{name: "trailing JSON", output: `{"sessions":[]} {}`, want: "trailing JSON"},
		{name: "missing sessions", output: `{}`, want: "missing sessions array"},
		{name: "null sessions", output: `{"sessions":null}`, want: "missing sessions array"},
		{name: "wrong sessions type", output: `{"sessions":{}}`, want: "decode sessions"},
		{name: "missing session field", output: `{"sessions":[{"name":"alpha","running":true,"default":false}]}`, want: "missing required field"},
		{name: "empty session name", output: `{"sessions":[{"name":"","running":true,"default":false,"socket_path":"/tmp/herdr.sock"}]}`, want: "empty name"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeHerdr(t, `printf '%s' "$HERDR_FAKE_OUTPUT"`)
			t.Setenv("HERDR_FAKE_OUTPUT", tc.output)
			_, err := New(nil, nil, nil).List(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("List error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestListReportsCommandFailure(t *testing.T) {
	fakeHerdr(t, `printf 'cannot enumerate sessions' >&2; exit 7`)

	_, err := New(nil, nil, nil).List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "herdr session list --json") || !strings.Contains(err.Error(), "cannot enumerate sessions") {
		t.Fatalf("List error = %v", err)
	}
}

func TestLaunchUsesProjectRootAndTerminalStreams(t *testing.T) {
	fakeHerdr(t, `
test "$#" -eq 2
test "$1" = --session
printf 'cwd=%s name=%s input=' "$PWD" "$2"
IFS= read -r input
printf '%s' "$input"
printf 'child stderr' >&2
`)
	projectRoot := t.TempDir()
	stdin := strings.NewReader("from terminal\n")
	var stdout, stderr bytes.Buffer

	err := New(stdin, &stdout, &stderr).Launch(context.Background(), projectRoot, "fledge-example-1234abcd")
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	wantStdout := "cwd=" + projectRoot + " name=fledge-example-1234abcd input=from terminal"
	if stdout.String() != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantStdout)
	}
	if stderr.String() != "child stderr" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestLaunchReportsFailure(t *testing.T) {
	fakeHerdr(t, `exit 8`)

	err := New(nil, nil, nil).Launch(context.Background(), t.TempDir(), "broken")
	if err == nil || !strings.Contains(err.Error(), "herdr --session broken") {
		t.Fatalf("Launch error = %v", err)
	}
}

func TestStop(t *testing.T) {
	fakeHerdr(t, `
test "$#" -eq 4
test "$1" = session
test "$2" = stop
test "$3" = managed
test "$4" = --json
`)

	if err := New(nil, nil, nil).Stop(context.Background(), "managed"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestStopReportsCommandFailure(t *testing.T) {
	fakeHerdr(t, `printf '{"error":"refused"}' >&2; exit 9`)

	err := New(nil, nil, nil).Stop(context.Background(), "managed")
	if err == nil || !strings.Contains(err.Error(), "herdr session stop managed --json") || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("Stop error = %v", err)
	}
}
