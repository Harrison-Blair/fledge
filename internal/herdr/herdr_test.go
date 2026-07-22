package herdr

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubHerdr(t *testing.T, script string) string {
	t.Helper()
	bin := t.TempDir()
	path := filepath.Join(bin, "herdr")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func listenUnix(t *testing.T) (string, func()) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	return sock, func() { ln.Close() }
}

func TestListAndFindSessions(t *testing.T) {
	stubHerdr(t, `
if [ "$1 $2 $3" = "session list --json" ]; then
  printf '%s' "$HERDR_LIST"
  exit 0
fi
exit 9
`)
	t.Setenv("HERDR_LIST", `{"sessions":[{"name":"first","running":false,"default":false,"socket_path":"/tmp/first"},{"name":"main","running":true,"default":true,"socket_path":"/tmp/main"}]}`)

	sessions, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 2 || sessions[1].Name != "main" || !sessions[1].Running || !sessions[1].Default {
		t.Fatalf("sessions = %+v", sessions)
	}
	for _, tc := range []struct {
		name, want string
		found      bool
	}{
		{name: "", want: "main", found: true},
		{name: "first", want: "first", found: true},
		{name: "missing", found: false},
	} {
		s, found, err := Find(tc.name)
		if err != nil || found != tc.found || s.Name != tc.want {
			t.Errorf("Find(%q) = %+v, %v, %v", tc.name, s, found, err)
		}
	}
}

func TestListReportsCommandAndJSONFailures(t *testing.T) {
	t.Run("command", func(t *testing.T) {
		stubHerdr(t, `printf 'list failed' >&2; exit 7`)
		_, err := List()
		if err == nil || !strings.Contains(err.Error(), "herdr session list") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("json", func(t *testing.T) {
		stubHerdr(t, `printf 'not-json'; exit 0`)
		_, err := List()
		if err == nil || !strings.Contains(err.Error(), "herdr session list") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestUpProbesSessionSocket(t *testing.T) {
	if Up("") || Up(filepath.Join(t.TempDir(), "absent.sock")) {
		t.Fatal("Up reported a missing socket")
	}
	sock, closeSocket := listenUnix(t)
	if !Up(sock) {
		t.Fatal("Up missed a live socket")
	}
	closeSocket()
	if Up(sock) {
		t.Fatal("Up reported a closed socket")
	}
}

func TestEnsureReusesLiveNamedAndDefaultSessions(t *testing.T) {
	sock, closeSocket := listenUnix(t)
	defer closeSocket()
	record := filepath.Join(t.TempDir(), "started")
	stubHerdr(t, `
if [ "$1 $2 $3" = "session list --json" ]; then
  printf '{"sessions":[{"name":"main","running":true,"default":true,"socket_path":"%s"}]}' "$HERDR_SOCKET"
  exit 0
fi
printf x > "$HERDR_RECORD"
exit 0
`)
	t.Setenv("HERDR_SOCKET", sock)
	t.Setenv("HERDR_RECORD", record)

	for _, name := range []string{"", "main"} {
		s, started, err := Ensure(name, []string{"EXTRA=value"}, t.TempDir())
		if err != nil || started || s.Name != "main" {
			t.Fatalf("Ensure(%q) = %+v, %v, %v", name, s, started, err)
		}
	}
	if _, err := os.Stat(record); !os.IsNotExist(err) {
		t.Fatalf("reuse launched herdr: %v", err)
	}
}

func TestEnsureMissingDefaultExplainsHowToStartIt(t *testing.T) {
	stubHerdr(t, `printf '{"sessions":[]}'; exit 0`)
	_, started, err := Ensure("", nil, t.TempDir())
	if err == nil || started || !strings.Contains(err.Error(), "no default herdr session running") {
		t.Fatalf("Ensure = started %v, err %v", started, err)
	}
}

func TestEnsureStartsNamedSessionWithDirectoryAndEnvironment(t *testing.T) {
	sock, closeSocket := listenUnix(t)
	defer closeSocket()
	record := t.TempDir()
	root := t.TempDir()
	stubHerdr(t, `
if [ "$1 $2 $3" = "session list --json" ]; then
  if [ -f "$HERDR_RECORD/started" ]; then
    printf '{"sessions":[{"name":"%s","running":true,"default":false,"socket_path":"%s"}]}' "$(sed -n '1p' "$HERDR_RECORD/started")" "$HERDR_SOCKET"
  else
    printf '{"sessions":[]}'
  fi
  exit 0
fi
if [ "$1" = "--session" ] && [ "$3" = "server" ]; then
  printf '%s\n%s\n%s\n%s\n' "$2" "$PWD" "$FLEDGE_FLOCK" "$CUSTOM_VALUE" > "$HERDR_RECORD/started"
  exit 0
fi
exit 8
`)
	t.Setenv("HERDR_SOCKET", sock)
	t.Setenv("HERDR_RECORD", record)

	s, started, err := Ensure("named", []string{"FLEDGE_FLOCK=alpha", "CUSTOM_VALUE=present"}, root)
	if err != nil || !started || s.Name != "named" || s.SocketPath != sock {
		t.Fatalf("Ensure = %+v, %v, %v", s, started, err)
	}
	data, err := os.ReadFile(filepath.Join(record, "started"))
	if err != nil {
		t.Fatal(err)
	}
	want := "named\n" + root + "\nalpha\npresent\n"
	if string(data) != want {
		t.Fatalf("start record = %q, want %q", data, want)
	}
}

func TestRecreateReplacesLiveSessionBeforeStarting(t *testing.T) {
	sock, closeSocket := listenUnix(t)
	defer closeSocket()
	record := t.TempDir()
	root := t.TempDir()
	stubHerdr(t, `
if [ "$1 $2 $3" = "session list --json" ]; then
  if [ -f "$HERDR_RECORD/live" ] || [ -f "$HERDR_RECORD/started" ]; then
    printf '{"sessions":[{"name":"managed","running":true,"default":false,"socket_path":"%s"}]}' "$HERDR_SOCKET"
  else
    printf '{"sessions":[]}'
  fi
  exit 0
fi
if [ "$1 $2" = "session stop" ]; then
  printf 'stop %s\n' "$3" >> "$HERDR_RECORD/calls"
  rm -f "$HERDR_RECORD/live"
  exit 0
fi
if [ "$1 $2" = "session delete" ]; then
  printf 'delete %s\n' "$3" >> "$HERDR_RECORD/calls"
  exit 0
fi
if [ "$1" = "--session" ] && [ "$3" = "server" ]; then
  printf 'start %s %s %s\n' "$2" "$PWD" "$FLEDGE_FLOCK" >> "$HERDR_RECORD/calls"
  touch "$HERDR_RECORD/started"
  exit 0
fi
exit 8
`)
	t.Setenv("HERDR_SOCKET", sock)
	t.Setenv("HERDR_RECORD", record)
	if err := os.WriteFile(filepath.Join(record, "live"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Recreate("managed", []string{"FLEDGE_FLOCK=flock1"}, root)
	if err != nil {
		t.Fatalf("Recreate: %v", err)
	}
	if s.Name != "managed" || s.SocketPath != sock {
		t.Fatalf("Recreate = %+v", s)
	}
	calls, err := os.ReadFile(filepath.Join(record, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	want := "stop managed\ndelete managed\nstart managed " + root + " flock1\n"
	if string(calls) != want {
		t.Fatalf("calls = %q, want %q", calls, want)
	}
}

func TestStopAndDeleteInvokeHerdrAndWrapFailures(t *testing.T) {
	record := filepath.Join(t.TempDir(), "calls")
	stubHerdr(t, `
printf '%s %s %s\n' "$1" "$2" "$3" >> "$HERDR_RECORD"
if [ "$3" = "bad" ]; then printf 'refused' >&2; exit 4; fi
exit 0
`)
	t.Setenv("HERDR_RECORD", record)

	if err := Stop("alpha"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := Delete("alpha"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := Stop("bad"); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("Stop failure = %v", err)
	}
	if err := Delete("bad"); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("Delete failure = %v", err)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []string{"session stop alpha", "session delete alpha", "session stop bad", "session delete bad"} {
		if !strings.Contains(string(data), call) {
			t.Errorf("calls %q missing %q", data, call)
		}
	}
}
