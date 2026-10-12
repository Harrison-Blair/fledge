package message

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// recorder keeps every request's params so a test can read the text of
// prompts whose doorbell names a store id it cannot predict.
type recorder struct {
	inner interface {
		Call(context.Context, string, any, any) error
	}
	prompts []string
}

func (r *recorder) Call(ctx context.Context, method string, params, result any) error {
	if method == "agent.prompt" {
		r.prompts = append(r.prompts, params.(map[string]any)["text"].(string))
	}
	return r.inner.Call(ctx, method, params, result)
}

// worker is a registered-able agent in pane with its own terminal.
func worker(pane, terminal, name string) herdr.AgentResult {
	return tasktest.Agent(pane, terminal, name)
}

func promptedAs(a herdr.AgentResult) herdr.AgentResult {
	return herdr.AgentResult{Type: "agent_prompted", Agent: a.Agent}
}

func senderOf(name, pane *string) *libagent.Sender {
	return &libagent.Sender{Name: name, Pane: pane, Kind: "named"}
}

func storeAt(t *testing.T, cwd string) *state.Store {
	t.Helper()
	return tasktest.Store(t, cwd)
}

// A registered recipient gets only the doorbell; the body is stored. Each of
// the three target forms finds the record.
func TestRegisteredRecipientGetsDoorbellNotBody(t *testing.T) {
	for _, form := range []string{"name", "pane", "id"} {
		t.Run(form, func(t *testing.T) {
			repo := identitytest.Repository(t)
			w := worker("w1:p3", "term_w", "worker")
			rec := tasktest.Register(t, repo, w)
			sel, target := selector.Selection{}, ""
			switch form {
			case "name":
				sel.Names, target = []string{"worker"}, "worker"
			case "pane":
				sel.Panes, target = []string{"w1:p3"}, "w1:p3"
			case "id":
				sel.IDs, target = []string{rec.ID}, "w1:p3"
			}
			c := herdrscript.Client(t, tasktest.Get(target, w), senderCall(), call{Method: "agent.prompt", Result: promptedAs(w)})
			c.Cwd = repo
			r := &recorder{inner: c.API}
			c.API = r
			out := run(context.Background(), c, Options{Selection: sel, Body: "SECRET body", BodySet: true}, nil, "m-0a1b2c")
			res, ok := out.Result.(Result)
			if out.Status != "success" || !ok || res.InboxID == nil || res.State != "rung" || res.MessageID != *res.InboxID {
				t.Fatalf("%+v", out)
			}
			stored, err := inbox.Get(storeAt(t, repo), *res.InboxID)
			if err != nil || stored.Body != "SECRET body" || stored.To != rec.ID || stored.Kind != inbox.Message || inbox.State(stored) != "rung" || stored.Rings[0].Pane != "w1:p3" {
				t.Fatalf("%+v %v", stored, err)
			}
			if len(r.prompts) != 1 || r.prompts[0] != inbox.Doorbell(stored, "") || strings.Contains(r.prompts[0], "SECRET") {
				t.Fatalf("%q", r.prompts)
			}
			if *stored.From.Name != "orchestrator" || *stored.From.Pane != "old:p1" || stored.From.ID != nil {
				t.Fatalf("%+v", stored.From)
			}
		})
	}
}

// An unregistered recipient in a repository still gets the header and body.
func TestUnregisteredRecipientGetsDirectBody(t *testing.T) {
	repo := identitytest.Repository(t)
	identitytest.Register(t, repo, worker("w1:p9", "term_other", "other").Agent)
	w := worker("w1:p3", "term_w", "worker")
	c := herdrscript.Client(t, tasktest.Get("worker", w), senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + "hi"}, Result: promptedAs(w)})
	c.Cwd = repo
	out := run(context.Background(), c, names("worker"), nil, "m-0a1b2c")
	res := out.Result.(Result)
	if out.Status != "success" || res.InboxID != nil || res.State != "direct" || res.MessageID != "m-0a1b2c" {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil || !strings.Contains(b.String(), `"inbox_id":null`) || !strings.Contains(b.String(), `"state":"direct"`) {
		t.Fatal(b.String(), err)
	}
	b.Reset()
	if err := out.Write(&b, false, Render); err != nil || !strings.Contains(b.String(), "has no Fledge record") {
		t.Fatal(b.String())
	}
	if all, _ := inbox.List(storeAt(t, repo)); len(all) != 0 {
		t.Fatal(all)
	}
}

// A blocked or not-ready registered recipient is deferred: stored, not rung,
// partial; the failed attempt is logged.
func TestBlockedRecipientIsDeferred(t *testing.T) {
	for _, code := range []string{"agent_blocked", "agent_not_ready"} {
		t.Run(code, func(t *testing.T) {
			repo := identitytest.Repository(t)
			w := worker("w1:p3", "term_w", "worker")
			tasktest.Register(t, repo, w)
			c := herdrscript.Client(t, tasktest.Get("worker", w), senderCall(), call{Method: "agent.prompt", Err: &herdr.Error{Code: code, Message: "dialog"}})
			c.Cwd = repo
			out := run(context.Background(), c, names("worker"), nil, "m-0a1b2c")
			res := out.Result.(Result)
			if out.Status != "partial" || out.Error == nil || out.Error.Code != code || res.State != "queued" || res.InboxID == nil || res.Submitted {
				t.Fatalf("%+v", out)
			}
			stored, _ := inbox.Get(storeAt(t, repo), *res.InboxID)
			if inbox.State(stored) != "queued" || len(stored.Rings) != 1 || stored.Rings[0].OK {
				t.Fatalf("%+v", stored)
			}
			var b bytes.Buffer
			out.Write(&b, false, Render)
			if !strings.Contains(b.String(), "stored") || !strings.Contains(b.String(), "supervisor") {
				t.Fatal(b.String())
			}
		})
	}
}

// A store failure for a registered recipient rejects the target with no
// prompt; the body never falls back to direct typing.
func TestStoreFailureRejectsRegisteredRecipient(t *testing.T) {
	repo := identitytest.Repository(t)
	w := worker("w1:p3", "term_w", "worker")
	rec := tasktest.Register(t, repo, w)
	restore := identitytest.ReadOnly(t, repo, rec.ID)
	defer restore()
	c := herdrscript.Client(t, tasktest.Get("worker", w), senderCall())
	c.Cwd = repo
	out := run(context.Background(), c, names("worker"), nil, "m-0a1b2c")
	if out.Status != "rejected" || out.Error == nil {
		t.Fatalf("%+v", out)
	}
}

// Fan-out stores one record per registered target with distinct ids; an
// unregistered target in the same fan-out is typed directly.
func TestFanOutStoresDistinctRecords(t *testing.T) {
	repo := identitytest.Repository(t)
	a, b, u := worker("w1:p3", "term_a", "a"), worker("w1:p4", "term_b", "b"), worker("w1:p5", "term_u", "u")
	tasktest.Register(t, repo, a)
	tasktest.Register(t, repo, b)
	c := herdrscript.Client(t, tasktest.Get("a", a), tasktest.Get("b", b), tasktest.Get("u", u), senderCall(),
		call{Method: "agent.prompt", Result: promptedAs(a)}, call{Method: "agent.prompt", Result: promptedAs(b)}, call{Method: "agent.prompt", Params: map[string]any{"target": "u", "text": header + "hi"}, Result: promptedAs(u)})
	c.Cwd = repo
	out := run(context.Background(), c, names("a", "b", "u"), nil, "m-0a1b2c")
	f := out.Result.(FanOut)
	if out.Status != "success" || len(f.Targets) != 3 {
		t.Fatalf("%+v", out)
	}
	ra, rb, ru := f.Targets[0], f.Targets[1], f.Targets[2]
	if ra.InboxID == nil || rb.InboxID == nil || *ra.InboxID == *rb.InboxID || *ra.MessageID != *ra.InboxID || *ra.State != "rung" {
		t.Fatalf("%+v %+v", ra, rb)
	}
	if ru.InboxID != nil || *ru.State != "direct" || *ru.MessageID != "m-0a1b2c" {
		t.Fatalf("%+v", ru)
	}
	data, _ := json.Marshal(ra)
	if !strings.Contains(string(data), `"inbox_id":"`+*ra.InboxID+`"`) || !strings.Contains(string(data), `"state":"rung"`) {
		t.Fatal(string(data))
	}
}

// A fan-out target blocked at sending is deferred, not rejected.
func TestFanOutDeferredRow(t *testing.T) {
	repo := identitytest.Repository(t)
	a, b := worker("w1:p3", "term_a", "a"), worker("w1:p4", "term_b", "b")
	tasktest.Register(t, repo, a)
	tasktest.Register(t, repo, b)
	c := herdrscript.Client(t, tasktest.Get("a", a), tasktest.Get("b", b), senderCall(),
		call{Method: "agent.prompt", Err: &herdr.Error{Code: "agent_blocked", Message: "dialog"}}, call{Method: "agent.prompt", Result: promptedAs(b)})
	c.Cwd = repo
	out := run(context.Background(), c, names("a", "b"), nil, "m-0a1b2c")
	f := out.Result.(FanOut)
	if out.Status != "partial" || f.Targets[0].Outcome != "deferred" || *f.Targets[0].State != "queued" || f.Targets[1].Outcome != "submitted" {
		t.Fatalf("%+v", out)
	}
	var buf bytes.Buffer
	out.Write(&buf, false, Render)
	if !strings.Contains(buf.String(), "stored for a") {
		t.Fatal(buf.String())
	}
}

// --reply-to links the reply and marks the original answered; only the
// original's recipient may use it.
func TestReplyToMarksOriginalAnswered(t *testing.T) {
	repo := identitytest.Repository(t)
	me := worker("old:p1", "term_me", "orchestrator")
	meRec := tasktest.Register(t, repo, me)
	w := worker("w1:p3", "term_w", "worker")
	wRec := tasktest.Register(t, repo, w)
	s := storeAt(t, repo)
	orig, _ := inbox.Create(s, inbox.Record{Kind: inbox.Message, From: inbox.From{ID: &wRec.ID, Pane: &w.Agent.PaneID}, To: meRec.ID, Body: "question"})
	c := herdrscript.Client(t, tasktest.Get("worker", w), tasktest.Get("old:p1", me), call{Method: "agent.prompt", Result: promptedAs(w)})
	c.Cwd = repo
	o := names("worker")
	o.ReplyTo = orig.ID
	out := run(context.Background(), c, o, nil, "m-0a1b2c")
	res := out.Result.(Result)
	if out.Status != "success" || res.InboxID == nil {
		t.Fatalf("%+v", out)
	}
	reply, _ := inbox.Get(s, *res.InboxID)
	answered, _ := inbox.Get(s, orig.ID)
	if reply.ReplyTo == nil || *reply.ReplyTo != orig.ID || *reply.From.ID != meRec.ID || answered.AnsweredAt == nil || *answered.AnsweredBy != reply.ID {
		t.Fatalf("%+v %+v", reply, answered)
	}
}

func TestReplyToRejectsNonRecipient(t *testing.T) {
	repo := identitytest.Repository(t)
	me := worker("old:p1", "term_me", "orchestrator")
	tasktest.Register(t, repo, me)
	w := worker("w1:p3", "term_w", "worker")
	wRec := tasktest.Register(t, repo, w)
	orig, _ := inbox.Create(storeAt(t, repo), inbox.Record{Kind: inbox.Message, To: wRec.ID, Body: "not mine"})
	c := herdrscript.Client(t, tasktest.Get("worker", w), tasktest.Get("old:p1", me))
	c.Cwd = repo
	o := names("worker")
	o.ReplyTo = orig.ID
	out := run(context.Background(), c, o, nil, "m-0a1b2c")
	if out.Status != "rejected" || out.Error.Code != "inbox_not_recipient" {
		t.Fatalf("%+v", out)
	}
	if all, _ := inbox.List(storeAt(t, repo)); len(all) != 1 {
		t.Fatal(all)
	}
}

func TestReplyToValidatesID(t *testing.T) {
	o := names("worker")
	o.ReplyTo = "nope"
	out := run(context.Background(), herdrscript.Client(t), o, nil, "m-0a1b2c")
	if out.ExitCode() != 2 {
		t.Fatalf("%+v", out)
	}
}

func TestRenderInboxResult(t *testing.T) {
	id := "0a1b2c3d"
	name, pane := "orchestrator", "wA:p1"
	r := Result{AgentRow: herdrscript.Row(), Submitted: true, MessageID: id, InboxID: &id, State: "rung", Sender: senderOf(&name, &pane)}
	var b bytes.Buffer
	if err := (cli.Outcome{Status: "success", Result: r}).Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "Message 0a1b2c3d stored for w1:p1 from orchestrator (wA:p1); doorbell rung.\n" {
		t.Fatalf("%q", got)
	}
}

// fakeStarter replaces the daemon starter, counting launches.
func fakeStarter(t *testing.T) *int {
	t.Helper()
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_SOCKET_PATH", "/nonexistent")
	t.Setenv("HERDR_SESSION", "")
	n := 0
	old := startSupervisor
	startSupervisor = func(string, string) error { n++; return nil }
	t.Cleanup(func() { startSupervisor = old })
	return &n
}

// A deferred message starts the supervisor; a rung one does not.
func TestDeferredMessageStartsSupervisor(t *testing.T) {
	for _, blocked := range []bool{true, false} {
		starts := fakeStarter(t)
		repo := identitytest.Repository(t)
		w := worker("w1:p3", "term_w", "worker")
		tasktest.Register(t, repo, w)
		prompt := call{Method: "agent.prompt", Result: promptedAs(w)}
		if blocked {
			prompt = call{Method: "agent.prompt", Err: &herdr.Error{Code: "agent_blocked", Message: "dialog"}}
		}
		c := herdrscript.Client(t, tasktest.Get("worker", w), senderCall(), prompt)
		c.Cwd = repo
		out := run(context.Background(), c, names("worker"), nil, "m-0a1b2c")
		want := 0
		if blocked {
			want = 1
		}
		started := false
		for _, e := range out.Effects {
			started = started || e == (cli.Effect{Action: "started", Kind: "supervisor"})
		}
		if *starts != want || started != blocked {
			t.Fatalf("blocked=%v: %d starts, effects %+v", blocked, *starts, out.Effects)
		}
	}
}
