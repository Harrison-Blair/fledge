package status

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/supervise"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func TestStatusReportsLiveDaemon(t *testing.T) {
	repo := identitytest.Repository(t)
	t.Setenv("HERDR_SESSION", "work")
	l, err := supervise.Acquire(supervise.Dir(repo, "work"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	now := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	l.Write(supervise.Status{PID: 77, Version: "v1.2.3", StartedAt: "2026-01-01T00:00:00Z", HeartbeatAt: "2026-01-01T00:00:55Z", Watched: []string{"aaaaaaaa", "bbbbbbbb"}})
	out := Run(context.Background(), Options{Cwd: repo, Now: now})
	r, ok := out.Result.(Result)
	if out.Error != nil || !ok || !r.Live || r.Session != "work" || r.Status.PID != 77 || len(r.Status.Watched) != 2 || r.Stale {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	out.Write(&b, false, Render)
	for _, want := range []string{"running", "pid 77", "version v1.2.3", "Watching 2: aaaaaaaa, bbbbbbbb"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %q in %q", want, b.String())
		}
	}
}

func TestStatusFlagsStaleHeartbeat(t *testing.T) {
	repo := identitytest.Repository(t)
	t.Setenv("HERDR_SESSION", "work")
	l, _ := supervise.Acquire(supervise.Dir(repo, "work"))
	defer l.Release()
	l.Write(supervise.Status{PID: 77, Version: "v1", HeartbeatAt: "2026-01-01T00:00:00Z"})
	out := Run(context.Background(), Options{Cwd: repo, Now: time.Date(2026, 1, 1, 0, 5, 0, 0, time.UTC)})
	var b bytes.Buffer
	out.Write(&b, false, Render)
	if !out.Result.(Result).Stale || !strings.Contains(b.String(), "heartbeat is stale") {
		t.Fatal(b.String())
	}
}

func TestStatusNotRunning(t *testing.T) {
	repo := identitytest.Repository(t)
	t.Setenv("HERDR_SESSION", "work")
	out := Run(context.Background(), Options{Cwd: repo, Now: time.Now()})
	var b bytes.Buffer
	out.Write(&b, false, Render)
	if out.Error != nil || out.Result.(Result).Live || b.String() != "Supervisor for session work: not running.\n" {
		t.Fatalf("%q %+v", b.String(), out.Error)
	}
}

func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render, cli.Outcome{Result: Result{Session: "s"}})
}
