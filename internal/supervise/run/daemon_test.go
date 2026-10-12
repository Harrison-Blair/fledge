package run

import (
	"bytes"
	"context"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

type call = herdrscript.Call

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// world is a repository with a boss and its spawned worker.
type world struct {
	repo        string
	store       *state.Store
	dir         string
	boss, work  identity.Record
	bossA, wrkA herdr.AgentDetails
	logs        *bytes.Buffer
}

func agent(pane, terminal, name, status string) herdr.AgentDetails {
	a := tasktest.Agent(pane, terminal, name).Agent
	a.AgentStatus = status
	return a
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{repo: identitytest.Repository(t), logs: &bytes.Buffer{}}
	w.bossA, w.wrkA = agent("w1:p1", "term_boss", "boss", "idle"), agent("w1:p3", "term_work", "worker", "working")
	w.boss = identitytest.Register(t, w.repo, w.bossA)
	w.work = identitytest.RegisterChild(t, w.repo, w.wrkA, w.boss.ID)
	w.store = tasktest.Store(t, w.repo)
	w.dir = filepath.Join(t.TempDir(), "supervise")
	return w
}

// daemon builds a daemon whose Herdr serves exactly calls.
func (w *world) daemon(t *testing.T, calls ...call) *daemon {
	t.Helper()
	c := herdrscript.Client(t, calls...)
	c.CallerPane, c.API = "", targetCheck{t: t, inner: c.API}
	d, err := newDaemon(c, w.store, w.dir, 5*time.Minute, log.New(w.logs, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// with returns a with status.
func with(a herdr.AgentDetails, status string) herdr.AgentDetails {
	a.AgentStatus = status
	return a
}

// prompt is a successful agent.prompt into pane.
func prompt(pane string) call {
	p := herdrscript.Pane(pane, "w1", "w1:t1")
	p.AgentStatus = "working"
	return call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}}
}

// targetCheck fails t unless each agent.prompt went to the pane its scripted
// result names and typed only a doorbell.
type targetCheck struct {
	t     *testing.T
	inner libagent.API
}

func (c targetCheck) Call(ctx context.Context, method string, params, result any) error {
	err := c.inner.Call(ctx, method, params, result)
	if method == "agent.prompt" && err == nil {
		p := params.(map[string]any)
		if got, want := p["target"], result.(*herdr.AgentResult).Agent.PaneID; got != want {
			c.t.Errorf("prompted %v, scripted for %s", got, want)
		}
		if text := p["text"].(string); !strings.HasPrefix(text, "ᛉ fledge inbox · ") || strings.Contains(text, "\n") {
			c.t.Errorf("typed %q, not a doorbell", text)
		}
	}
	return err
}

func wakes(t *testing.T, s *state.Store, to string) []inbox.Record {
	t.Helper()
	all, err := inbox.ForRecipient(s, to)
	if err != nil {
		t.Fatal(err)
	}
	var ws []inbox.Record
	for _, m := range all {
		if m.Kind == inbox.Wake {
			ws = append(ws, m)
		}
	}
	return ws
}

func TestReconcileWatchesChildrenOwnersAndRecipients(t *testing.T) {
	w := newWorld(t)
	loner := agent("w1:p5", "term_loner", "loner", "idle")
	owner := agent("w1:p6", "term_owner", "owner", "idle")
	reader := agent("w1:p7", "term_reader", "reader", "idle")
	identitytest.Register(t, w.repo, loner)
	ownerRec := identitytest.Register(t, w.repo, owner)
	readerRec := identitytest.Register(t, w.repo, reader)
	tasktest.Seed(t, w.repo, task.Record{Title: "t", Status: task.Assigned, Owner: &ownerRec.ID})
	inbox.Create(w.store, inbox.Record{Kind: inbox.Message, To: readerRec.ID, Body: "b", Rings: nil})
	// The queued record for reader is rung during the reconcile.
	d := w.daemon(t, herdrscript.List(w.bossA, w.wrkA, loner, owner, reader), prompt("w1:p7"))
	if err := d.reconcile(context.Background(), t0); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(d.panes(), " ")
	if got != "w1:p3 w1:p6 w1:p7" {
		t.Fatalf("watched %q", got)
	}
}

func TestBlockedWakesParentOncePerEpisode(t *testing.T) {
	w := newWorld(t)
	d := w.daemon(t, herdrscript.List(w.bossA, w.wrkA), prompt("w1:p1"), prompt("w1:p1"))
	ctx := context.Background()
	if err := d.reconcile(ctx, t0); err != nil {
		t.Fatal(err)
	}
	d.status(ctx, "w1:p3", "blocked", t0.Add(time.Second))
	d.status(ctx, "w1:p3", "blocked", t0.Add(2*time.Second))
	ws := wakes(t, w.store, w.boss.ID)
	if len(ws) != 1 || !strings.Contains(ws[0].Body, "fledge agent read --pane w1:p3") || !strings.Contains(ws[0].Body, "fledge agent send --pane w1:p3 --key") || inbox.State(ws[0]) != "rung" {
		t.Fatalf("%+v", ws)
	}
	// A new episode wakes again.
	d.status(ctx, "w1:p3", "working", t0.Add(3*time.Second))
	d.status(ctx, "w1:p3", "blocked", t0.Add(4*time.Second))
	if ws := wakes(t, w.store, w.boss.ID); len(ws) != 2 {
		t.Fatalf("%d wakes", len(ws))
	}
}

// A restarted daemon reads the episode state and does not repeat the wake.
func TestBlockedWakeSurvivesRestart(t *testing.T) {
	w := newWorld(t)
	blocked := with(w.wrkA, "blocked")
	first := w.daemon(t, herdrscript.List(w.bossA, blocked), prompt("w1:p1"))
	if err := first.reconcile(context.Background(), t0); err != nil {
		t.Fatal(err)
	}
	second := w.daemon(t, herdrscript.List(w.bossA, blocked))
	if err := second.reconcile(context.Background(), t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if ws := wakes(t, w.store, w.boss.ID); len(ws) != 1 {
		t.Fatalf("%d wakes", len(ws))
	}
}

// Leaving blocked rings the worker's queued records once each.
func TestLeavingBlockedRingsQueuedRecords(t *testing.T) {
	w := newWorld(t)
	m, _ := inbox.Create(w.store, inbox.Record{Kind: inbox.Message, To: w.work.ID, Body: "deferred"})
	blocked := with(w.wrkA, "blocked")
	d := w.daemon(t, herdrscript.List(w.bossA, blocked), prompt("w1:p1"), prompt("w1:p3"))
	ctx := context.Background()
	if err := d.reconcile(ctx, t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := inbox.Get(w.store, m.ID); inbox.State(got) != "queued" {
		t.Fatalf("rung while blocked: %+v", got)
	}
	d.status(ctx, "w1:p3", "idle", t0.Add(time.Second))
	if got, _ := inbox.Get(w.store, m.ID); inbox.State(got) != "rung" || len(got.Rings) != 1 || got.Rings[0].Pane != "w1:p3" {
		t.Fatalf("%+v", got)
	}
}

func TestExitWakesUnlessFledgeEndedTheRecord(t *testing.T) {
	for _, ended := range []bool{false, true} {
		w := newWorld(t)
		calls := []call{herdrscript.List(w.bossA, w.wrkA)}
		if !ended {
			calls = append(calls, prompt("w1:p1"))
		}
		d := w.daemon(t, calls...)
		ctx := context.Background()
		if err := d.reconcile(ctx, t0); err != nil {
			t.Fatal(err)
		}
		tasktest.Seed(t, w.repo, task.Record{Title: "Fix it", Status: task.Assigned, Owner: &w.work.ID})
		if ended {
			if err := identity.End(w.store, w.work.ID); err != nil {
				t.Fatal(err)
			}
		}
		d.exitedPane(ctx, "w1:p3")
		d.exitedPane(ctx, "w1:p3")
		ws := wakes(t, w.store, w.boss.ID)
		if ended && (len(ws) != 0 || !strings.Contains(w.logs.String(), "suppressed")) {
			t.Fatalf("ended: %+v\n%s", ws, w.logs)
		}
		if !ended && (len(ws) != 1 || !strings.Contains(ws[0].Body, "fledge task list --owner "+w.work.ID) || !strings.Contains(ws[0].Body, "Fix it")) {
			t.Fatalf("%+v", ws)
		}
		if len(d.watched) != 0 {
			t.Fatal(d.watched)
		}
	}
}

func TestQuietWakesAfterThirtySecondsWithAssignedTask(t *testing.T) {
	w := newWorld(t)
	tasktest.Seed(t, w.repo, task.Record{Title: "Fix it", Status: task.Assigned, Owner: &w.work.ID})
	d := w.daemon(t, herdrscript.List(w.bossA, w.wrkA), prompt("w1:p1"))
	ctx := context.Background()
	d.reconcile(ctx, t0)
	d.status(ctx, "w1:p3", "idle", t0)
	d.tick(ctx, t0.Add(29*time.Second))
	if ws := wakes(t, w.store, w.boss.ID); len(ws) != 0 {
		t.Fatal("early quiet wake")
	}
	// done after idle is the same episode.
	d.status(ctx, "w1:p3", "done", t0.Add(10*time.Second))
	d.tick(ctx, t0.Add(31*time.Second))
	d.tick(ctx, t0.Add(60*time.Second))
	ws := wakes(t, w.store, w.boss.ID)
	if len(ws) != 1 || !strings.Contains(ws[0].Body, "fledge agent read --name worker") || !strings.Contains(ws[0].Body, "Fix it") {
		t.Fatalf("%+v", ws)
	}
	if !d.ep.Quiet[w.work.ID] {
		t.Fatal("quiet episode not recorded")
	}
	// Working ends the episode.
	d.status(ctx, "w1:p3", "working", t0.Add(61*time.Second))
	if d.ep.Quiet[w.work.ID] {
		t.Fatal("quiet episode not cleared")
	}
}

func TestQuietSkipsWorkerThatReportedOrOwnsNoTask(t *testing.T) {
	// No assigned task.
	w := newWorld(t)
	d := w.daemon(t, herdrscript.List(w.bossA, with(w.wrkA, "idle")))
	ctx := context.Background()
	d.reconcile(ctx, t0)
	d.tick(ctx, t0.Add(31*time.Second))
	if ws := wakes(t, w.store, w.boss.ID); len(ws) != 0 {
		t.Fatal("woke without a task")
	}
	// A message to the parent after the last doorbell.
	w = newWorld(t)
	tasktest.Seed(t, w.repo, task.Record{Title: "Fix it", Status: task.Assigned, Owner: &w.work.ID})
	in, _ := inbox.Create(w.store, inbox.Record{Kind: inbox.Assignment, To: w.work.ID, Body: "brief"})
	inbox.AddRing(w.store, in.ID, "w1:p3", nil)
	inbox.MarkRead(w.store, in.ID)
	pane := "w1:p3"
	inbox.Create(w.store, inbox.Record{Kind: inbox.Message, From: inbox.From{ID: &w.work.ID, Pane: &pane}, To: w.boss.ID, Body: "question"})
	inbox.MarkRead(w.store, mustLast(t, w.store, w.boss.ID).ID)
	d = w.daemon(t, herdrscript.List(w.bossA, with(w.wrkA, "idle")))
	d.reconcile(ctx, t0)
	d.tick(ctx, t0.Add(31*time.Second))
	if ws := wakes(t, w.store, w.boss.ID); len(ws) != 0 {
		t.Fatalf("woke after the worker reported: %+v", ws)
	}
}

func mustLast(t *testing.T, s *state.Store, to string) inbox.Record {
	t.Helper()
	all, err := inbox.ForRecipient(s, to)
	if err != nil || len(all) == 0 {
		t.Fatal(all, err)
	}
	return all[len(all)-1]
}

// An unread record is rung once more after ack_timeout, then its registered
// sender is told after another; a wake is only logged.
func TestUnreadRecordsAreReRungThenReported(t *testing.T) {
	w := newWorld(t)
	pane := "w1:p1"
	msg, _ := inbox.Create(w.store, inbox.Record{Kind: inbox.Message, From: inbox.From{ID: &w.boss.ID, Pane: &pane}, To: w.work.ID, Body: "hello"})
	inbox.AddRing(w.store, msg.ID, "w1:p3", nil)
	wk, _ := inbox.Create(w.store, inbox.Record{Kind: inbox.Wake, From: inbox.Supervisor(), To: w.work.ID, Body: "wake"})
	inbox.AddRing(w.store, wk.ID, "w1:p3", nil)
	rung, _ := inbox.Get(w.store, msg.ID)
	first := firstRing(rung)
	ctx := context.Background()
	// t1: both re-rung; t2: the message's sender is told, the wake logged.
	d := w.daemon(t,
		herdrscript.List(w.bossA, w.wrkA),
		herdrscript.List(w.bossA, w.wrkA), prompt("w1:p3"), prompt("w1:p3"),
		herdrscript.List(w.bossA, w.wrkA), prompt("w1:p1"),
	)
	d.reconcile(ctx, first.Add(time.Minute))
	d.reconcile(ctx, first.Add(5*time.Minute+time.Second))
	d.reconcile(ctx, first.Add(10*time.Minute+2*time.Second))
	got, _ := inbox.Get(w.store, msg.ID)
	if len(got.Rings) != 2 {
		t.Fatalf("%+v", got.Rings)
	}
	ws := wakes(t, w.store, w.boss.ID)
	if len(ws) != 1 || !strings.Contains(ws[0].Body, msg.ID) || !strings.Contains(ws[0].Body, "still unread") {
		t.Fatalf("%+v", ws)
	}
	if !strings.Contains(w.logs.String(), "unacknowledged "+wk.ID) {
		t.Fatal(w.logs.String())
	}
}

func TestMovedAndDroppedPanes(t *testing.T) {
	w := newWorld(t)
	d := w.daemon(t, herdrscript.List(w.bossA, w.wrkA))
	d.reconcile(context.Background(), t0)
	d.moved("w1:p3", "w2:p1")
	if d.watched["w2:p1"] == nil || d.watched["w1:p3"] != nil {
		t.Fatal(d.watched)
	}
	d.drop("w2:p1")
	if len(d.watched) != 0 {
		t.Fatal(d.watched)
	}
}

func TestIdleExitAfterSixtySeconds(t *testing.T) {
	w := newWorld(t)
	d := w.daemon(t)
	if d.idle(t0) || d.idle(t0.Add(59*time.Second)) || !d.idle(t0.Add(60*time.Second)) {
		t.Fatal("idle timing")
	}
	// An unread record keeps it busy.
	inbox.Create(w.store, inbox.Record{Kind: inbox.Message, To: w.work.ID, Body: "x"})
	if d.idle(t0.Add(2 * time.Minute)) {
		t.Fatal("idle with unread record")
	}
}

// An agent that stops qualifying while still live is unwatched, not exited:
// no wake goes to its task's creator.
func TestUnqualifiedAgentIsUnwatchedNotExited(t *testing.T) {
	w := newWorld(t)
	owner := agent("w1:p6", "term_owner", "owner", "working")
	ownerRec := identitytest.Register(t, w.repo, owner)
	id := tasktest.Seed(t, w.repo, task.Record{Title: "t", Status: task.Assigned, Owner: &ownerRec.ID, CreatedBy: &w.boss.ID})
	d := w.daemon(t, herdrscript.List(w.bossA, w.wrkA, owner), herdrscript.List(w.bossA, w.wrkA, owner))
	ctx := context.Background()
	d.reconcile(ctx, t0)
	if d.watched["w1:p6"] == nil {
		t.Fatal("owner not watched")
	}
	if _, err := task.Update(w.store, id, func(r *task.Record) error { r.Status = task.Completed; return nil }); err != nil {
		t.Fatal(err)
	}
	d.reconcile(ctx, t0.Add(30*time.Second))
	if d.watched["w1:p6"] != nil || d.ep.Exited[ownerRec.ID] {
		t.Fatalf("watched %v exited %v", d.watched, d.ep.Exited)
	}
	if ws := wakes(t, w.store, w.boss.ID); len(ws) != 0 || strings.Contains(w.logs.String(), "wake exited") {
		t.Fatalf("%+v\n%s", ws, w.logs)
	}
}

// A watched worker gone from agent.list exited unseen.
func TestVanishedWorkerIsExited(t *testing.T) {
	w := newWorld(t)
	d := w.daemon(t, herdrscript.List(w.bossA, w.wrkA), herdrscript.List(w.bossA), prompt("w1:p1"))
	ctx := context.Background()
	d.reconcile(ctx, t0)
	d.reconcile(ctx, t0.Add(30*time.Second))
	if ws := wakes(t, w.store, w.boss.ID); len(ws) != 1 || !strings.Contains(ws[0].Body, "exited") {
		t.Fatalf("%+v", ws)
	}
}
