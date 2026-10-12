package cleanup

import (
	"context"
	"slices"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/supervise"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// retiring is a caller with one finished worker that cleanup stops.
type retiring struct {
	r              repo
	caller, worker herdr.AgentDetails
	callerRec, rec identity.Record
}

func newRetiring(t *testing.T) retiring {
	t.Helper()
	r := newRepo(t)
	caller := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
	callerRec := r.register(t, caller, nil, "adopt", nil)
	w := agent("w3:p1", "w3", "t_worker", "worker", "idle")
	rec := r.register(t, w, &callerRec.ID, "spawn", nil)
	tasktest.Seed(t, r.root, task.Record{Title: "t", Owner: &rec.ID, Status: task.Verified})
	return retiring{r, caller, w, callerRec, rec}
}

func (x retiring) run(t *testing.T, o Options) cli.Outcome {
	t.Helper()
	// No worker has a checkout, so the plan lists no worktrees.
	calls := []call{herdrscript.Get(x.caller.PaneID, x.caller), herdrscript.List(x.caller, x.worker)}
	if !o.DryRun {
		calls = append(calls, herdrscript.Get(x.worker.PaneID, x.worker), call{Method: "pane.close", Result: herdrscript.OK()})
	}
	return Run(context.Background(), client(t, x.r, calls...), o)
}

// fakeStop replaces the daemon stop, recording the directories stopped.
func fakeStop(t *testing.T, l *supervise.Lock) *[]string {
	t.Helper()
	var stopped []string
	old := stopSupervisor
	stopSupervisor = func(_ context.Context, dir string) error {
		stopped = append(stopped, dir)
		l.Release()
		return nil
	}
	t.Cleanup(func() { stopSupervisor = old })
	return &stopped
}

func TestCleanupArchivesRetiredWorkersInbox(t *testing.T) {
	x := newRetiring(t)
	s := tasktest.Store(t, x.r.root)
	pane := x.worker.PaneID
	to, _ := inbox.Create(s, inbox.Record{Kind: inbox.Message, To: x.rec.ID, Body: "to the worker"})
	from, _ := inbox.Create(s, inbox.Record{Kind: inbox.Message, From: inbox.From{ID: &x.rec.ID, Pane: &pane}, To: x.callerRec.ID, Body: "from the worker"})
	kept, _ := inbox.Create(s, inbox.Record{Kind: inbox.Message, To: x.callerRec.ID, Body: "unrelated"})
	out := x.run(t, Options{})
	if out.Error != nil || worker(out.Result.(Result), x.rec.ID).Outcome != "done" {
		t.Fatalf("%+v", out)
	}
	all, _ := inbox.List(s)
	if len(all) != 1 || all[0].ID != kept.ID {
		t.Fatalf("%+v", all)
	}
	for _, id := range []string{to.ID, from.ID} {
		if _, err := inbox.Get(s, id); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(out.Effects, cli.Effect{Action: "archived", Kind: "inbox", ID: id}) {
			t.Fatalf("%+v", out.Effects)
		}
	}
}

// The daemon is stopped only when nothing it watches survives the cleanup.
func TestCleanupStopsSupervisorOnlyWhenNothingRemainsWatched(t *testing.T) {
	for _, alsoCaller := range []bool{false, true} {
		x := newRetiring(t)
		dir := supervise.Dir(x.r.root, "")
		l, err := supervise.Acquire(dir)
		if err != nil {
			t.Fatal(err)
		}
		watched := []string{x.rec.ID}
		if alsoCaller {
			watched = append(watched, x.callerRec.ID)
		}
		l.Write(supervise.Status{PID: 1, Version: "v", Watched: watched})
		stopped := fakeStop(t, l)
		out := x.run(t, Options{})
		if out.Error != nil {
			t.Fatalf("%+v", out)
		}
		stop := slices.Contains(out.Effects, cli.Effect{Action: "stopped", Kind: "supervisor"})
		if alsoCaller == (len(*stopped) == 1) || stop == alsoCaller {
			t.Fatalf("alsoCaller=%v: stopped %v, effects %+v", alsoCaller, *stopped, out.Effects)
		}
		l.Release()
	}
}

func TestCleanupDryRunLeavesInboxAndSupervisor(t *testing.T) {
	x := newRetiring(t)
	s := tasktest.Store(t, x.r.root)
	inbox.Create(s, inbox.Record{Kind: inbox.Message, To: x.rec.ID, Body: "to the worker"})
	l, _ := supervise.Acquire(supervise.Dir(x.r.root, ""))
	defer l.Release()
	l.Write(supervise.Status{PID: 1, Version: "v", Watched: []string{x.rec.ID}})
	stopped := fakeStop(t, l)
	out := x.run(t, Options{DryRun: true})
	if all, _ := inbox.List(s); out.Error != nil || len(all) != 1 || len(*stopped) != 0 {
		t.Fatalf("%+v %+v %v", out, all, *stopped)
	}
}
