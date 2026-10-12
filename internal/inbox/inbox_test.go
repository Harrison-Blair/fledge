package inbox

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	libinbox "github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

var boss = tasktest.Agent("w1:p1", "term_boss", "boss")

// setup registers boss as the caller and stores records for it and for
// someone else; it returns the repository and boss's record.
func setup(t *testing.T) (string, identity.Record) {
	t.Helper()
	repo := identitytest.Repository(t)
	me := tasktest.Register(t, repo, boss)
	s := tasktest.Store(t, repo)
	pane, name, from := "w1:p3", "worker", "bbbbbbbb"
	mk := func(to, body string, read bool) {
		r, err := libinbox.Create(s, libinbox.Record{Kind: libinbox.Message, From: libinbox.From{ID: &from, Name: &name, Pane: &pane}, To: to, Body: body})
		if err == nil && read {
			_, err = libinbox.MarkRead(s, r.ID)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	mk(me.ID, "already read", true)
	mk(me.ID, "first unread\nsecond line", false)
	mk("cccccccc", "not mine", false)
	mk(me.ID, "\x1b[31mred\x1b[0m unread", false)
	return repo, me
}

func TestListShowsUnreadByDefault(t *testing.T) {
	repo, _ := setup(t)
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{Width: 200, Now: time.Now().Add(90 * time.Second)})
	r, ok := out.Result.(Result)
	if out.Error != nil || !ok || len(r.Records) != 2 || r.Records[0].Body != "first unread\nsecond line" || r.Records[0].State != "queued" {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) != 5 || strings.Join(strings.Fields(lines[0]), " ") != "ID STATE KIND FROM TASK AGE" {
		t.Fatalf("%q", b.String())
	}
	if f := strings.Fields(lines[1]); len(f) != 7 || f[1] != "queued" || f[2] != "message" || f[3] != "worker" || f[4] != "(w1:p3)" || f[5] != "-" || f[6] != "1m" {
		t.Fatalf("%q", lines[1])
	}
	if lines[2] != "  first unread" || lines[4] != "  red unread" {
		t.Fatalf("%q", b.String())
	}
}

func TestListAllIncludesReadAndJSON(t *testing.T) {
	repo, _ := setup(t)
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{All: true, Width: 80, Now: time.Now()})
	r := out.Result.(Result)
	if len(r.Records) != 3 || r.Records[2].Body != "already read" || r.Records[2].State != "read" {
		t.Fatalf("%+v", r)
	}
	var b bytes.Buffer
	out.Write(&b, true, Render)
	var decoded struct {
		Result struct {
			Records []map[string]any `json:"records"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b.Bytes(), &decoded); err != nil || len(decoded.Result.Records) != 3 || decoded.Result.Records[0]["state"] != "queued" || decoded.Result.Records[0]["body"] == nil {
		t.Fatal(b.String(), err)
	}
}

func TestListTruncatesPreviewToWidth(t *testing.T) {
	repo, me := setup(t)
	libinbox.Create(tasktest.Store(t, repo), libinbox.Record{Kind: libinbox.Message, To: me.ID, Body: strings.Repeat("x", 300)})
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{Width: 40, Now: time.Now()})
	var b bytes.Buffer
	out.Write(&b, false, Render)
	for _, line := range strings.Split(b.String(), "\n") {
		if strings.HasPrefix(line, "  ") && len([]rune(line)) > 40 {
			t.Fatalf("%q longer than 40", line)
		}
	}
	if !strings.Contains(b.String(), "…") {
		t.Fatal(b.String())
	}
}

func TestListEmpty(t *testing.T) {
	repo := identitytest.Repository(t)
	tasktest.Register(t, repo, boss)
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{Width: 80, Now: time.Now()})
	var b bytes.Buffer
	out.Write(&b, false, Render)
	if out.Error != nil || b.String() != "No unread inbox messages.\n" {
		t.Fatalf("%q %+v", b.String(), out.Error)
	}
}

func TestListRequiresRegisteredCaller(t *testing.T) {
	repo := identitytest.Repository(t)
	tasktest.Register(t, repo, tasktest.Agent("w1:p3", "term_w", "worker"))
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{Width: 80, Now: time.Now()})
	if out.Error == nil || out.Error.Code != "caller_unregistered" {
		t.Fatalf("%+v", out)
	}
	// A repository with no Fledge state at all is the same failure.
	out = Run(context.Background(), tasktest.Client(t, identitytest.Repository(t), "w1:p1"), Options{Width: 80, Now: time.Now()})
	if out.Error == nil || out.Error.Code != "caller_unregistered" {
		t.Fatalf("%+v", out)
	}
}

func TestAge(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		ago  time.Duration
		want string
	}{{5 * time.Second, "5s"}, {3 * time.Minute, "3m"}, {2 * time.Hour, "2h"}, {50 * time.Hour, "2d"}} {
		if got := Age(now.Add(-tc.ago).Format(time.RFC3339Nano), now); got != tc.want {
			t.Errorf("%v: got %s want %s", tc.ago, got, tc.want)
		}
	}
	if got := Age("garbage", now); got != "-" {
		t.Fatal(got)
	}
}

func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render, cli.Outcome{Result: Result{Records: []libinbox.View{libinbox.NewView(libinbox.Record{ID: "0a1b2c3d", Kind: libinbox.Message, Body: "b", CreatedAt: "x"})}}})
}

// Columns line up across rows even with preview lines between them.
func TestTableAlignsColumns(t *testing.T) {
	long, short := "a-very-long-sender-name", "x"
	pa, pb := "w1:p3", "w1:p4"
	now := time.Now()
	rows := []libinbox.View{
		libinbox.NewView(libinbox.Record{ID: "0a1b2c3d", Kind: libinbox.Message, From: libinbox.From{Name: &long, Pane: &pa}, Body: "a", CreatedAt: now.Format(time.RFC3339Nano)}),
		libinbox.NewView(libinbox.Record{ID: "1a1b2c3d", Kind: libinbox.Wake, From: libinbox.From{Name: &short, Pane: &pb}, Body: "b", CreatedAt: now.Format(time.RFC3339Nano)}),
	}
	lines := strings.Split(Table(rows, 80, now), "\n")
	header, second := lines[0], lines[3]
	task := func(line string) int { return strings.Index(line, "  -  ") + 2 }
	if strings.Index(header, "TASK") != task(second) || task(lines[1]) != task(second) {
		t.Fatalf("misaligned:\n%s", strings.Join(lines, "\n"))
	}
}

// A linked worktree reads the primary checkout's state.
func TestListFromLinkedWorktree(t *testing.T) {
	repo, _ := setup(t)
	gittest.Commit(t, repo)
	linked := filepath.Join(t.TempDir(), "linked")
	gittest.Git(t, repo, "worktree", "add", "-q", "-b", "side", linked)
	out := Run(context.Background(), tasktest.Client(t, linked, "w1:p1", tasktest.Get("w1:p1", boss)), Options{Now: time.Now()})
	if out.Error != nil || len(out.Result.(Result).Records) != 2 {
		t.Fatalf("%+v", out)
	}
}
