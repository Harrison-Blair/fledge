package get

import (
	"bytes"
	"context"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestGetShowsVerifiedCommitAndLanding(t *testing.T) {
	repo := identitytest.Repository(t)
	p := tasktest.Ptr[string]
	hash, merge := "0123456789abcdef0123456789abcdef01234567", "89abcdef0123456789abcdef0123456789abcdef"
	id := tasktest.Seed(t, repo, task.Record{Title: "T", Brief: "b", Status: task.Landed, CreatedAt: "2026-01-01T00:00:00Z",
		DeliveryMode: task.ModePR, BaseBranch: p("dev"), Branch: p("feat/x"), Verifier: p("bbbbbbbb"), VerifiedAt: p("2026-01-01T00:03:00Z"), VerifiedCommit: &hash,
		LandedAt: p("2026-01-01T00:04:00Z"), LandedBy: p("cccccccc"), MergeCommit: &merge, TargetBranch: p("dev"), PRURL: p("https://example.com/pr/1"), LandStep: p("confirm")})
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id})
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	want := "id: " + id + "\ntitle: T\nstatus: landed\nowner: -\n" +
		"delivery mode: pr\nbase branch: dev\n" +
		"created: 2026-01-01T00:00:00Z by an unregistered caller\n" +
		"verified: 2026-01-01T00:03:00Z by bbbbbbbb\n" +
		"verified commit: " + hash + "\n" +
		"branch: feat/x\npull request: https://example.com/pr/1\n" +
		"landed: 2026-01-01T00:04:00Z by cccccccc into dev as " + merge + "\n" +
		"brief:\n  b\n"
	if b.String() != want {
		t.Fatalf("%q\nwant %q", b.String(), want)
	}
}

func TestGetShowsLandStepOfARefusedLanding(t *testing.T) {
	repo := identitytest.Repository(t)
	p := tasktest.Ptr[string]
	id := tasktest.Seed(t, repo, task.Record{Title: "T", Brief: "b", Status: task.Verified, CreatedAt: "2026-01-01T00:00:00Z",
		DeliveryMode: task.ModeNone, VerifiedAt: p("2026-01-01T00:03:00Z"), LandStep: p("checks")})
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id})
	var b bytes.Buffer
	out.Write(&b, false, Render)
	want := "id: " + id + "\ntitle: T\nstatus: verified\nowner: -\n" +
		"created: 2026-01-01T00:00:00Z by an unregistered caller\n" +
		"verified: 2026-01-01T00:03:00Z by an unregistered caller\n" +
		"land step: checks\n" +
		"brief:\n  b\n"
	if b.String() != want {
		t.Fatalf("%q\nwant %q", b.String(), want)
	}
}
