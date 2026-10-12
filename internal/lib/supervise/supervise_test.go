package supervise

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDirIsPerSessionAndSafe(t *testing.T) {
	root := "/repo"
	for session, want := range map[string]string{
		"default":   "/repo/.fledge/state/supervise/default",
		"":          "/repo/.fledge/state/supervise/_none",
		"a/../b c":  "/repo/.fledge/state/supervise/a_.._b_c",
		"..":        "/repo/.fledge/state/supervise/_..",
		"work.2026": "/repo/.fledge/state/supervise/work.2026",
	} {
		if got := Dir(root, session); got != want {
			t.Errorf("%q: got %s want %s", session, got, want)
		}
	}
}

func TestLockIsExclusiveAndReadable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "s")
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(dir); !errors.Is(err, ErrHeld) {
		t.Fatalf("second acquire: %v", err)
	}
	st := Status{PID: 42, Version: "v1", StartedAt: "t0", HeartbeatAt: "t1", Watched: []string{"aaaaaaaa"}}
	if err := l.Write(st); err != nil {
		t.Fatal(err)
	}
	// A shorter rewrite leaves no stale bytes behind.
	st.Watched = nil
	if err := l.Write(st); err != nil {
		t.Fatal(err)
	}
	got, live, err := Read(dir)
	if err != nil || !live || got.PID != 42 || got.Version != "v1" || len(got.Watched) != 0 {
		t.Fatalf("%+v %v %v", got, live, err)
	}
	l.Release()
	if _, live, err := Read(dir); err != nil || live {
		t.Fatalf("live after release: %v %v", live, err)
	}
	l2, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	l2.Release()
}

func TestReadMissingDirectoryIsNotLive(t *testing.T) {
	if _, live, err := Read(filepath.Join(t.TempDir(), "missing")); err != nil || live {
		t.Fatal(live, err)
	}
}

func TestEnsureStartsOnlyWhenFree(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "s")
	starts := 0
	start := func() error { starts++; return nil }
	started, err := ensure(context.Background(), dir, "v1", start, nil)
	if err != nil || !started || starts != 1 {
		t.Fatal(started, err, starts)
	}
	l, _ := Acquire(dir)
	defer l.Release()
	l.Write(Status{PID: os.Getpid(), Version: "v1"})
	started, err = ensure(context.Background(), dir, "v1", start, nil)
	if err != nil || started || starts != 1 {
		t.Fatal(started, err, starts)
	}
}

// A live daemon of another version is stopped and replaced.
func TestEnsureReplacesOtherVersion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "s")
	l, _ := Acquire(dir)
	l.Write(Status{PID: 4242, Version: "old"})
	stopped := 0
	stop := func(ctx context.Context, d string) error {
		stopped++
		l.Release()
		return nil
	}
	starts := 0
	started, err := ensure(context.Background(), dir, "new", func() error { starts++; return nil }, stop)
	if err != nil || !started || stopped != 1 || starts != 1 {
		t.Fatal(started, err, stopped, starts)
	}
}

// helperHold is a subprocess that holds the lock until SIGTERM.
func TestHelperHold(t *testing.T) {
	dir := os.Getenv("SUPERVISE_HOLD_DIR")
	if dir == "" {
		t.Skip("helper process only")
	}
	l, err := Acquire(dir)
	if err != nil {
		os.Exit(3)
	}
	l.Write(Status{PID: os.Getpid(), Version: "v1"})
	time.Sleep(time.Minute)
	os.Exit(4)
}

func TestStopSignalsTheHolderAndWaits(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "s")
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperHold$")
	cmd.Env = append(os.Environ(), "SUPERVISE_HOLD_DIR="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if st, live, _ := Read(dir); live && st.PID == cmd.Process.Pid {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper never took the lock")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := Stop(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if _, live, _ := Read(dir); live {
		t.Fatal("still live after Stop")
	}
	err := <-done
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGTERM {
		t.Fatalf("helper ended with %v", err)
	}
}

func TestStopWithoutDaemonIsNoop(t *testing.T) {
	if err := Stop(context.Background(), filepath.Join(t.TempDir(), "s")); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchRefusesTestBinary(t *testing.T) {
	err := Launch(t.TempDir(), "s")
	if err == nil || !strings.Contains(err.Error(), "test binary") {
		t.Fatal(err)
	}
}

func TestChildEnvKeepsOnlySessionAndSocket(t *testing.T) {
	env := childEnv([]string{"PATH=/bin", "HOME=/h", "HERDR_PANE_ID=w1:p1", "HERDR_SOCKET_PATH=/s", "HERDR_SESSION=x", "HERDR_ENV=1", "SECRET=1"})
	got := strings.Join(env, " ")
	if got != "PATH=/bin HOME=/h HERDR_SOCKET_PATH=/s HERDR_SESSION=x HERDR_ENV=1" {
		t.Fatal(got)
	}
}

func TestEpisodesRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "s")
	e, err := LoadEpisodes(dir)
	if err != nil || len(e.Blocked) != 0 {
		t.Fatal(e, err)
	}
	e.Blocked["aaaaaaaa"] = true
	e.Quiet["bbbbbbbb"] = true
	e.Exited["cccccccc"] = true
	e.Rerung["0a1b2c3d"] = "t"
	e.Unacknowledged["0a1b2c3d"] = true
	if err := e.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := LoadEpisodes(dir)
	if err != nil || !got.Blocked["aaaaaaaa"] || !got.Quiet["bbbbbbbb"] || !got.Exited["cccccccc"] || got.Rerung["0a1b2c3d"] != "t" || !got.Unacknowledged["0a1b2c3d"] {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestEnsureForNeedsHerdrAndSkipsTestBinary(t *testing.T) {
	repo := t.TempDir()
	gitInit(t, repo)
	t.Setenv("HERDR_ENV", "")
	called := false
	start := func(string, string) error { called = true; return nil }
	if started, err := EnsureFor(context.Background(), repo, start); started || err != nil || called {
		t.Fatal(started, err, called)
	}
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_SOCKET_PATH", "/nonexistent")
	t.Setenv("HERDR_SESSION", "s")
	if started, err := EnsureFor(context.Background(), repo, start); !started || err != nil || !called {
		t.Fatal(started, err, called)
	}
	if started, err := EnsureFor(context.Background(), repo, Launch); started || err != nil {
		t.Fatal(started, err)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
}
