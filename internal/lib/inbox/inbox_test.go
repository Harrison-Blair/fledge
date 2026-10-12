package inbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func store(t *testing.T) *state.Store {
	t.Helper()
	s, err := identity.OpenStore(context.Background(), identitytest.Repository(t), &cli.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func named(id, name, pane string) inbox.From {
	return inbox.From{ID: &id, Name: &name, Pane: &pane}
}

func TestCreateStoresOneRecordPerCallWithStoreIDs(t *testing.T) {
	s := store(t)
	a, err := inbox.Create(s, inbox.Record{Kind: inbox.Message, From: named("aaaaaaaa", "orchestrator", "w1:p1"), To: "bbbbbbbb", Body: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := inbox.Create(s, inbox.Record{Kind: inbox.Message, To: "bbbbbbbb", Body: "again"})
	if err != nil {
		t.Fatal(err)
	}
	if !state.ValidID(a.ID) || a.ID == b.ID || a.CreatedAt == "" || a.Rings == nil {
		t.Fatalf("%+v %+v", a, b)
	}
	got, err := inbox.Get(s, a.ID)
	if err != nil || got.Body != "hello" || got.To != "bbbbbbbb" || *got.From.Name != "orchestrator" {
		t.Fatalf("%+v %v", got, err)
	}
	if inbox.State(got) != "queued" {
		t.Fatal(inbox.State(got))
	}
}

// The stored JSON carries exactly the I1 fields, with nulls for unset ones.
func TestRecordJSONShape(t *testing.T) {
	data, err := json.Marshal(inbox.Record{ID: "0a1b2c3d", Kind: inbox.Wake, From: inbox.Supervisor(), To: "bbbbbbbb", Body: "b", CreatedAt: "t", Rings: []inbox.Ring{}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"0a1b2c3d","kind":"wake","from":{"id":null,"name":"fledge supervisor","pane":null},"to":"bbbbbbbb","task":null,"body":"b","created_at":"t","rings":[],"read_at":null,"reply_to":null,"answered_at":null,"answered_by":null}`
	if string(data) != want {
		t.Fatalf("got  %s\nwant %s", data, want)
	}
}

func TestStateFollowsRingsReadAndAnswer(t *testing.T) {
	s := store(t)
	r, _ := inbox.Create(s, inbox.Record{Kind: inbox.Message, To: "bbbbbbbb", Body: "x"})
	r, err := inbox.AddRing(s, r.ID, "w1:p3", errors.New("agent_blocked: dialog"))
	if err != nil || inbox.State(r) != "queued" || len(r.Rings) != 1 || r.Rings[0].OK || *r.Rings[0].Error != "agent_blocked: dialog" || r.Rings[0].Pane != "w1:p3" {
		t.Fatalf("%+v %v", r, err)
	}
	if r, err = inbox.AddRing(s, r.ID, "w1:p3", nil); err != nil || inbox.State(r) != "rung" || !r.Rings[1].OK || r.Rings[1].Error != nil {
		t.Fatalf("%+v %v", r, err)
	}
	if !inbox.Rung(r) || inbox.LastRing(r) == nil {
		t.Fatal(r)
	}
	r.ReadAt = tasktest.Ptr("t")
	if inbox.State(r) != "read" {
		t.Fatal(inbox.State(r))
	}
	r.AnsweredAt = tasktest.Ptr("t")
	if inbox.State(r) != "answered" {
		t.Fatal(inbox.State(r))
	}
}

func TestDoorbellNamesSenderAndNeverTheBody(t *testing.T) {
	id := "0a1b2c3d"
	pane, name, rec := "w1:p1", "orchestrator", "aaaaaaaa"
	for _, tc := range []struct {
		r     inbox.Record
		title string
		want  string
	}{
		{inbox.Record{ID: id, Kind: inbox.Message, From: inbox.From{ID: &rec, Name: &name, Pane: &pane}, Body: "SECRET"}, "",
			"ᛉ fledge inbox · message from orchestrator (w1:p1) · id 0a1b2c3d · read: fledge inbox read --id 0a1b2c3d"},
		{inbox.Record{ID: id, Kind: inbox.Message, From: inbox.From{ID: &rec, Pane: &pane}, Body: "SECRET"}, "",
			"ᛉ fledge inbox · message from unnamed agent (w1:p1) · id 0a1b2c3d · read: fledge inbox read --id 0a1b2c3d"},
		{inbox.Record{ID: id, Kind: inbox.Message, From: inbox.From{Name: tasktest.Ptr(""), Pane: &pane}, Body: "SECRET"}, "",
			"ᛉ fledge inbox · message from unnamed agent (w1:p1) · id 0a1b2c3d · read: fledge inbox read --id 0a1b2c3d"},
		{inbox.Record{ID: id, Kind: inbox.Message, From: inbox.From{Pane: &pane}, Body: "SECRET"}, "",
			"ᛉ fledge inbox · message from pane w1:p1 · id 0a1b2c3d · read: fledge inbox read --id 0a1b2c3d"},
		{inbox.Record{ID: id, Kind: inbox.Wake, From: inbox.Supervisor(), Body: "SECRET"}, "",
			"ᛉ fledge inbox · wake from fledge supervisor · id 0a1b2c3d · read: fledge inbox read --id 0a1b2c3d"},
		{inbox.Record{ID: id, Kind: inbox.Assignment, From: inbox.From{ID: &rec, Name: &name, Pane: &pane}, Task: tasktest.Ptr("1234abcd"), Body: "SECRET"}, "Build it",
			"ᛉ fledge inbox · assignment from orchestrator (w1:p1) · id 0a1b2c3d · read: fledge inbox read --id 0a1b2c3d · task: 1234abcd · Build it"},
		{inbox.Record{ID: id, Kind: inbox.Message, Body: "SECRET"}, "",
			"ᛉ fledge inbox · message from unknown sender · id 0a1b2c3d · read: fledge inbox read --id 0a1b2c3d"},
	} {
		if got := inbox.Doorbell(tc.r, tc.title); got != tc.want {
			t.Errorf("got  %q\nwant %q", got, tc.want)
		}
	}
}

// A title or name with terminal controls or a newline cannot break the
// one-line doorbell.
func TestDoorbellIsOneCleanLine(t *testing.T) {
	name, pane := "evil\x1b[31m\nname", "w1:p1"
	got := inbox.Doorbell(inbox.Record{ID: "0a1b2c3d", Kind: inbox.Assignment, From: inbox.From{Name: &name, Pane: &pane}, Task: tasktest.Ptr("1234abcd")}, "two\nlines\x1b]0;x\x07")
	if strings.ContainsAny(got, "\n\x1b\x07") {
		t.Fatalf("%q", got)
	}
}

func TestForRecipientOrdersUnreadFirstThenNewestLast(t *testing.T) {
	s := store(t)
	mk := func(to, body string, read bool) inbox.Record {
		r, err := inbox.Create(s, inbox.Record{Kind: inbox.Message, To: to, Body: body})
		if err != nil {
			t.Fatal(err)
		}
		if read {
			if r, err = inbox.MarkRead(s, r.ID); err != nil {
				t.Fatal(err)
			}
		}
		return r
	}
	mk("bbbbbbbb", "old-read", true)
	mk("bbbbbbbb", "unread-1", false)
	mk("cccccccc", "other", false)
	mk("bbbbbbbb", "unread-2", false)
	mk("bbbbbbbb", "new-read", true)
	all, err := inbox.ForRecipient(s, "bbbbbbbb")
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, r := range all {
		bodies = append(bodies, r.Body)
	}
	if got := strings.Join(bodies, ","); got != "unread-1,unread-2,old-read,new-read" {
		t.Fatal(got)
	}
}

func TestMarkReadKeepsTheFirstReadTime(t *testing.T) {
	s := store(t)
	r, _ := inbox.Create(s, inbox.Record{Kind: inbox.Message, To: "bbbbbbbb", Body: "x"})
	first, err := inbox.MarkRead(s, r.ID)
	if err != nil || first.ReadAt == nil {
		t.Fatal(first, err)
	}
	again, err := inbox.MarkRead(s, r.ID)
	if err != nil || *again.ReadAt != *first.ReadAt {
		t.Fatal(again, err)
	}
}

func TestAnswerSetsAnsweredFieldsOnlyForTheRecipient(t *testing.T) {
	s := store(t)
	r, _ := inbox.Create(s, inbox.Record{Kind: inbox.Message, To: "bbbbbbbb", Body: "x"})
	if _, err := inbox.CheckRecipient(s, r.ID, "cccccccc"); err == nil {
		t.Fatal("non-recipient accepted")
	} else if code, _ := cli.Coded(err); code != "inbox_not_recipient" {
		t.Fatal(err)
	}
	if _, err := inbox.CheckRecipient(s, "ffffffff", "bbbbbbbb"); err == nil {
		t.Fatal("missing accepted")
	} else if code, _ := cli.Coded(err); code != "inbox_not_found" {
		t.Fatal(err)
	}
	if _, err := inbox.Answer(s, r.ID, "cccccccc", "dddddddd"); err == nil {
		t.Fatal("non-recipient answered")
	}
	got, err := inbox.Answer(s, r.ID, "bbbbbbbb", "dddddddd")
	if err != nil || got.AnsweredAt == nil || *got.AnsweredBy != "dddddddd" || inbox.State(got) != "answered" {
		t.Fatal(got, err)
	}
}

func TestNowIsRFC3339(t *testing.T) {
	if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z$`).MatchString(inbox.Now()) {
		t.Fatal(inbox.Now())
	}
}

func TestArchiveMovesRecordsToOrFromRetiredAgents(t *testing.T) {
	s := store(t)
	to, _ := inbox.Create(s, inbox.Record{Kind: inbox.Message, To: "bbbbbbbb", Body: "to"})
	from, _ := inbox.Create(s, inbox.Record{Kind: inbox.Message, From: named("bbbbbbbb", "w", "w1:p3"), To: "aaaaaaaa", Body: "from"})
	kept, _ := inbox.Create(s, inbox.Record{Kind: inbox.Message, To: "aaaaaaaa", Body: "kept"})
	archived, err := inbox.Archive(s, []string{"bbbbbbbb"})
	if err != nil || len(archived) != 2 {
		t.Fatal(archived, err)
	}
	all, _ := inbox.List(s)
	if len(all) != 1 || all[0].ID != kept.ID {
		t.Fatal(all)
	}
	for _, id := range []string{to.ID, from.ID} {
		if _, err := inbox.Get(s, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFromSenderKeepsEachSenderForm(t *testing.T) {
	name, pane, id := "orchestrator", "w1:p1", "aaaaaaaa"
	for _, tc := range []struct {
		s    libagent.Sender
		id   *string
		want string
	}{
		{libagent.Sender{Name: &name, Pane: &pane, Kind: "named"}, &id, "orchestrator (w1:p1)"},
		{libagent.Sender{Pane: &pane, Kind: "unnamed"}, nil, "unnamed agent (w1:p1)"},
		{libagent.Sender{Pane: &pane, Kind: "pane"}, nil, "pane w1:p1"},
		{libagent.Sender{Kind: "unknown"}, nil, "unknown sender"},
	} {
		f := inbox.FromSender(tc.s, tc.id)
		if got := f.String(); got != tc.want || f.ID != tc.id {
			t.Errorf("%+v: got %q want %q", tc.s, got, tc.want)
		}
		data, _ := json.Marshal(f)
		var back inbox.From
		json.Unmarshal(data, &back)
		if back.String() != tc.want {
			t.Errorf("round trip %s: got %q", data, back.String())
		}
	}
}
