package attention

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/supervise"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func agent(pane, terminal, name, status string) herdr.AgentDetails {
	a := tasktest.Agent(pane, terminal, name).Agent
	a.AgentStatus = status
	return a
}

type world struct {
	repo  string
	me    identity.Record
	meA   herdr.AgentDetails
	list  []herdr.AgentDetails
	kids  map[string]identity.Record
	other identity.Record
}

// newWorld registers a caller with one child per status, plus a stranger.
func newWorld(t *testing.T) world {
	t.Helper()
	t.Setenv("HERDR_SESSION", "")
	w := world{repo: identitytest.Repository(t), kids: map[string]identity.Record{}}
	w.meA = agent("w1:p1", "term_me", "orchestrator", "working")
	w.me = identitytest.Register(t, w.repo, w.meA)
	w.list = []herdr.AgentDetails{w.meA}
	for i, status := range []string{"blocked", "working", "idle", "done", "unknown", "quietidle"} {
		live := status
		if status == "quietidle" {
			live = "idle"
		}
		a := agent("w2:p"+string(rune('1'+i)), "term_"+status, status, live)
		w.kids[status] = identitytest.RegisterChild(t, w.repo, a, w.me.ID)
		w.list = append(w.list, a)
	}
	stranger := agent("w3:p1", "term_stranger", "stranger", "blocked")
	w.other = identitytest.Register(t, w.repo, stranger)
	w.list = append(w.list, stranger)
	return w
}

func (w world) run(t *testing.T) cli.Outcome {
	t.Helper()
	c := tasktest.Client(t, w.repo, "w1:p1", herdrscript.List(w.list...))
	return Run(context.Background(), c, Options{Width: 120, Now: time.Now()})
}

func TestAttentionGroupsWorkersInboxAndTasks(t *testing.T) {
	w := newWorld(t)
	s := tasktest.Store(t, w.repo)
	inbox.Create(s, inbox.Record{Kind: inbox.Wake, From: inbox.Supervisor(), To: w.me.ID, Body: "worker is blocked"})
	read, _ := inbox.Create(s, inbox.Record{Kind: inbox.Message, To: w.me.ID, Body: "old"})
	inbox.MarkRead(s, read.ID)
	done := tasktest.Seed(t, w.repo, task.Record{Title: "Ready to verify", Status: task.Completed, CreatedBy: &w.me.ID, Owner: tasktest.Ptr(w.kids["idle"].ID)})
	tasktest.Seed(t, w.repo, task.Record{Title: "Verified", Status: task.Verified, CreatedBy: &w.me.ID})
	tasktest.Seed(t, w.repo, task.Record{Title: "Someone else's", Status: task.Completed, CreatedBy: &w.other.ID})
	// A live daemon reports quietidle as quiet.
	dir := supervise.Dir(w.repo, "")
	l, err := supervise.Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	l.Write(supervise.Status{PID: 1, Version: "v7", HeartbeatAt: time.Now().UTC().Format(time.RFC3339Nano)})
	ep, _ := supervise.LoadEpisodes(dir)
	ep.Quiet[w.kids["quietidle"].ID] = true
	ep.Save(dir)

	out := w.run(t)
	r, ok := out.Result.(Result)
	if out.Error != nil || !ok {
		t.Fatalf("%+v", out)
	}
	if len(r.Inbox) != 1 || r.Inbox[0].Body != "worker is blocked" {
		t.Fatalf("%+v", r.Inbox)
	}
	groups := map[string]string{}
	for _, wk := range r.Workers {
		groups[wk.ID] = wk.Group
	}
	for status, rec := range w.kids {
		want := status
		if status == "quietidle" {
			want = "quiet"
		}
		if groups[rec.ID] != want {
			t.Errorf("%s: got %q", status, groups[rec.ID])
		}
	}
	if _, ok := groups[w.other.ID]; ok || len(r.Workers) != 6 {
		t.Fatalf("%+v", r.Workers)
	}
	if len(r.Verify) != 1 || r.Verify[0].ID != done {
		t.Fatalf("%+v", r.Verify)
	}
	if !r.Supervisor.Live || r.Supervisor.Version != "v7" {
		t.Fatalf("%+v", r.Supervisor)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	text := b.String()
	for _, want := range []string{"Inbox: 1 unread", "worker is blocked", "Workers:", "  blocked: blocked (" + w.kids["blocked"].ID + ") in w2:p1", "  quiet: quietidle", "Tasks to verify:", "  " + done + " Ready to verify", "Supervisor: running, version v7"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	// Groups appear in the fixed order.
	if strings.Index(text, "blocked:") > strings.Index(text, "quiet:") || strings.Index(text, "quiet:") > strings.Index(text, "working:") {
		t.Fatal(text)
	}
	b.Reset()
	out.Write(&b, true, Render)
	var decoded map[string]any
	if err := json.Unmarshal(b.Bytes(), &decoded); err != nil || decoded["result"].(map[string]any)["supervisor"] == nil {
		t.Fatal(b.String(), err)
	}
}

// Without a live daemon there is no quiet group: the worker shows idle.
func TestAttentionWithoutDaemon(t *testing.T) {
	w := newWorld(t)
	dir := supervise.Dir(w.repo, "")
	ep, _ := supervise.LoadEpisodes(dir)
	ep.Quiet[w.kids["quietidle"].ID] = true
	ep.Save(dir)
	r := w.run(t).Result.(Result)
	for _, wk := range r.Workers {
		if wk.Group == "quiet" {
			t.Fatalf("%+v", wk)
		}
	}
	var b bytes.Buffer
	w.run(t).Write(&b, false, Render)
	if !strings.Contains(b.String(), "Supervisor: not running") || !strings.Contains(b.String(), "Inbox: 0 unread") {
		t.Fatal(b.String())
	}
}

// Attention never writes the store.
func TestAttentionIsReadOnly(t *testing.T) {
	w := newWorld(t)
	check := identitytest.ReadOnly(t, w.repo, w.me.ID)
	if out := w.run(t); out.Error != nil {
		t.Fatalf("%+v", out)
	}
	check()
}

func TestAttentionRequiresRegisteredCaller(t *testing.T) {
	w := newWorld(t)
	c := tasktest.Client(t, w.repo, "w9:p9", herdrscript.List(w.list...))
	out := Run(context.Background(), c, Options{Now: time.Now()})
	if out.Error == nil || out.Error.Code != "caller_unregistered" {
		t.Fatalf("%+v", out)
	}
}

func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render, cli.Outcome{Result: Result{Workers: []Worker{{ID: "aaaaaaaa", Group: "idle"}}}})
}
