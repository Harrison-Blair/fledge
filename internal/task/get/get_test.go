package get

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestGetShowsFullRecord(t *testing.T) {
	repo := identitytest.Repository(t)
	p := tasktest.Ptr[string]
	id := tasktest.Seed(t, repo, task.Record{Title: "Fix it", Brief: "line one\nline two\n", Status: task.Verified, Owner: p("aaaaaaaa"),
		Result: p("fixed"), Verifier: p("bbbbbbbb"), VerificationNote: p("ok"), Forced: true, CreatedAt: "2026-01-01T00:00:00Z", CreatedBy: p("bbbbbbbb"),
		AssignedAt: p("2026-01-01T00:01:00Z"), CompletedAt: p("2026-01-01T00:02:00Z"), VerifiedAt: p("2026-01-01T00:03:00Z"),
		Delivery: &task.Delivery{MessageID: "m-0a1b2c", Pane: "w1:p3", DeliveredAt: p("2026-01-01T00:01:01Z")}})
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id})
	if out.Error != nil || out.Operation != "task.get" || !reflect.DeepEqual(out.Result, tasktest.Load(t, repo, id)) {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	want := "id: " + id + "\ntitle: Fix it\nstatus: verified\nowner: aaaaaaaa\n" +
		"created: 2026-01-01T00:00:00Z by bbbbbbbb\n" +
		"assigned: 2026-01-01T00:01:00Z\n" +
		"delivery: message m-0a1b2c to w1:p3, delivered 2026-01-01T00:01:01Z\n" +
		"completed: 2026-01-01T00:02:00Z\n" +
		"verified: 2026-01-01T00:03:00Z by bbbbbbbb (forced)\n" +
		"brief:\n  line one\n  line two\n" +
		"result:\n  fixed\n" +
		"verification note:\n  ok\n"
	if b.String() != want {
		t.Fatalf("%q\nwant %q", b.String(), want)
	}
}

func TestGetSparseRecord(t *testing.T) {
	repo := identitytest.Repository(t)
	p := tasktest.Ptr[string]
	id := tasktest.Seed(t, repo, task.Record{Title: "T", Brief: "b", Status: task.Cancelled, CreatedAt: "2026-01-01T00:00:00Z",
		AssignedAt: p("2026-01-01T00:01:00Z"), Owner: p("aaaaaaaa"), CancelledAt: p("2026-01-01T00:05:00Z"), CancelReason: p("dup"),
		Delivery: &task.Delivery{MessageID: "m-0a1b2c", Pane: "w1:p3", Error: p("agent_blocked: approval")}})
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id})
	var b bytes.Buffer
	out.Write(&b, false, Render)
	want := "id: " + id + "\ntitle: T\nstatus: cancelled\nowner: aaaaaaaa\n" +
		"created: 2026-01-01T00:00:00Z by an unregistered caller\n" +
		"assigned: 2026-01-01T00:01:00Z\n" +
		"delivery: message m-0a1b2c to w1:p3, failed: agent_blocked: approval\n" +
		"cancelled: 2026-01-01T00:05:00Z (dup)\n" +
		"brief:\n  b\n"
	if b.String() != want {
		t.Fatalf("%q\nwant %q", b.String(), want)
	}
}

func TestGetMissingAndInvalid(t *testing.T) {
	repo := identitytest.Repository(t)
	if out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: "0123abcd"}); out.Error == nil || out.Error.Code != "task_not_found" {
		t.Fatalf("%+v", out.Error)
	}
	if out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{}); out.Error == nil || out.Error.Code != "invalid_input" {
		t.Fatalf("%+v", out.Error)
	}
}
