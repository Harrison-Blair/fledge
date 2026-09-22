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
		Delivery:               &task.Delivery{MessageID: "m-0a1b2c", Pane: "w1:p3", DeliveredAt: p("2026-01-01T00:01:01Z")},
		CompletionNotification: &task.CompletionNotification{Recipient: "bbbbbbbb", MessageID: "m-abcdef", Pane: p("w1:p1"), DeliveredAt: p("2026-01-01T00:02:01Z")}})
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id})
	if out.Error != nil || out.Operation != "task.get" || !reflect.DeepEqual(out.Result, Result{Record: tasktest.Load(t, repo, id)}) {
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
		"completion notification: message m-abcdef to bbbbbbbb in w1:p1, delivered 2026-01-01T00:02:01Z\n" +
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
		Delivery:               &task.Delivery{MessageID: "m-0a1b2c", Pane: "w1:p3", Error: p("agent_blocked: approval")},
		CompletionNotification: &task.CompletionNotification{Recipient: "bbbbbbbb", MessageID: "m-abcdef", Error: p("transport_error: lost"), Uncertain: true}})
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id})
	var b bytes.Buffer
	out.Write(&b, false, Render)
	want := "id: " + id + "\ntitle: T\nstatus: cancelled\nowner: aaaaaaaa\n" +
		"created: 2026-01-01T00:00:00Z by an unregistered caller\n" +
		"assigned: 2026-01-01T00:01:00Z\n" +
		"delivery: message m-0a1b2c to w1:p3, failed: agent_blocked: approval\n" +
		"completion notification: message m-abcdef to bbbbbbbb, outcome unknown: transport_error: lost\n" +
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

func TestGetShowsParentAndSubtaskProgress(t *testing.T) {
	repo := identitytest.Repository(t)
	top := tasktest.Seed(t, repo, task.Record{Title: "top", Brief: "b", Status: task.Created, CreatedAt: "2026-01-01T00:00:00Z"})
	goal := tasktest.Seed(t, repo, task.Record{Title: "goal", Brief: "b", Status: task.Assigned, Parent: &top, CreatedAt: "2026-01-01T00:00:00Z"})
	for _, status := range []string{task.Verified, task.Completed, task.Created, task.Cancelled} {
		child := tasktest.Seed(t, repo, task.Record{Title: status, Status: status, Parent: &goal})
		tasktest.Seed(t, repo, task.Record{Title: "grandchild", Status: task.Verified, Parent: &child})
	}
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: goal})
	if r, ok := out.Result.(Result); out.Error != nil || !ok || r.Progress == nil || *r.Progress != (task.Progress{Verified: 1, Total: 3, Cancelled: 1}) {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	out.Write(&b, false, Render)
	want := "id: " + goal + "\ntitle: goal\nstatus: assigned\nowner: -\n" +
		"parent: " + top + "\n" +
		"subtasks: 1/3 verified, 1 cancelled\n" +
		"created: 2026-01-01T00:00:00Z by an unregistered caller\n" +
		"brief:\n  b\n"
	if b.String() != want {
		t.Fatalf("%q\nwant %q", b.String(), want)
	}
}
