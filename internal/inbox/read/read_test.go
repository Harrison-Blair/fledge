package read

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

var boss = tasktest.Agent("w1:p1", "term_boss", "boss")

type fixture struct {
	repo                                  string
	me                                    identity.Record
	registered, unregistered, wake, other inbox.Record
}

func setup(t *testing.T) fixture {
	t.Helper()
	repo := identitytest.Repository(t)
	me := tasktest.Register(t, repo, boss)
	s := tasktest.Store(t, repo)
	pane, name, from := "w1:p3", "worker", "bbbbbbbb"
	mk := func(r inbox.Record) inbox.Record {
		r, err := inbox.Create(s, r)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	return fixture{
		repo:         repo,
		me:           me,
		registered:   mk(inbox.Record{Kind: inbox.Assignment, From: inbox.From{ID: &from, Name: &name, Pane: &pane}, To: me.ID, Task: tasktest.Ptr("1234abcd"), Body: "brief \x1b[31mred\x1b[0m"}),
		unregistered: mk(inbox.Record{Kind: inbox.Message, From: inbox.From{Pane: tasktest.Ptr("w1:p9")}, To: me.ID, Body: "from a plain pane"}),
		wake:         mk(inbox.Record{Kind: inbox.Wake, From: inbox.Supervisor(), To: me.ID, Body: "worker is blocked"}),
		other:        mk(inbox.Record{Kind: inbox.Message, To: "cccccccc", Body: "not mine"}),
	}
}

func TestReadAllUnreadOldestFirstAndMarksRead(t *testing.T) {
	f := setup(t)
	out := Run(context.Background(), tasktest.Client(t, f.repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{})
	r, ok := out.Result.(Result)
	if out.Error != nil || !ok || len(r.Records) != 3 || r.Records[0].ID != f.registered.ID || r.Records[2].ID != f.wake.ID {
		t.Fatalf("%+v", out)
	}
	for _, v := range r.Records {
		if v.ReadAt == nil || v.State != "read" {
			t.Fatalf("%+v", v)
		}
	}
	s := tasktest.Store(t, f.repo)
	if got, _ := inbox.Get(s, f.other.ID); got.ReadAt != nil {
		t.Fatal("marked another agent's record read")
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	text := b.String()
	for _, want := range []string{
		"assignment " + f.registered.ID + " from worker (w1:p3) · task 1234abcd",
		"brief red\n",
		"Reply: fledge agent message --id bbbbbbbb --reply-to " + f.registered.ID + ` --body "..."`,
		"Reply: fledge agent message --pane w1:p9 --reply-to " + f.unregistered.ID + ` --body "..."`,
		"wake " + f.wake.ID + " from fledge supervisor",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b") || strings.Count(text, "Reply:") != 2 {
		t.Fatalf("%q", text)
	}
	// Nothing is unread now.
	again := Run(context.Background(), tasktest.Client(t, f.repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{})
	b.Reset()
	again.Write(&b, false, Render)
	if len(again.Result.(Result).Records) != 0 || b.String() != "No unread inbox messages.\n" {
		t.Fatalf("%q", b.String())
	}
}

func TestReadByIDKeepsFirstReadTime(t *testing.T) {
	f := setup(t)
	first := Run(context.Background(), tasktest.Client(t, f.repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{IDs: []string{f.wake.ID}})
	second := Run(context.Background(), tasktest.Client(t, f.repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{IDs: []string{f.wake.ID}})
	a, b := first.Result.(Result).Records, second.Result.(Result).Records
	if first.Error != nil || second.Error != nil || len(a) != 1 || len(b) != 1 || *a[0].ReadAt != *b[0].ReadAt {
		t.Fatalf("%+v %+v", first, second)
	}
	if got, _ := inbox.Get(tasktest.Store(t, f.repo), f.registered.ID); got.ReadAt != nil {
		t.Fatal("read an unrequested record")
	}
}

func TestReadRefusesAnotherAgentsRecord(t *testing.T) {
	f := setup(t)
	out := Run(context.Background(), tasktest.Client(t, f.repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{IDs: []string{f.wake.ID, f.other.ID}})
	if out.Error == nil || out.Error.Code != "inbox_not_recipient" {
		t.Fatalf("%+v", out)
	}
	if got, _ := inbox.Get(tasktest.Store(t, f.repo), f.wake.ID); got.ReadAt != nil {
		t.Fatal("marked read before refusing")
	}
}

func TestReadRejectsUnknownAndMalformedIDs(t *testing.T) {
	f := setup(t)
	out := Run(context.Background(), tasktest.Client(t, f.repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{IDs: []string{"ffffffff"}})
	if out.Error == nil || out.Error.Code != "inbox_not_found" {
		t.Fatalf("%+v", out)
	}
	out = Run(context.Background(), herdrscript.Client(t), Options{IDs: []string{"nope"}})
	if out.ExitCode() != 2 {
		t.Fatalf("%+v", out)
	}
}

func TestReadRequiresRegisteredCaller(t *testing.T) {
	f := setup(t)
	stranger := tasktest.Agent("w1:p8", "term_s", "stranger")
	out := Run(context.Background(), tasktest.Client(t, f.repo, "w1:p8", tasktest.Get("w1:p8", stranger)), Options{})
	if out.Error == nil || out.Error.Code != "caller_unregistered" {
		t.Fatalf("%+v", out)
	}
}

func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render, cli.Outcome{Result: Result{Records: []inbox.View{inbox.NewView(inbox.Record{ID: "0a1b2c3d", Kind: inbox.Message, Body: "b"})}}})
}
