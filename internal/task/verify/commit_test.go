package verify

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func verifyCommit(t *testing.T, repo, id, rev string) (task.Record, string, *string) {
	t.Helper()
	return verifyWith(t, tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), repo, id, rev)
}

func verifyWith(t *testing.T, c libagent.Client, repo, id, rev string) (task.Record, string, *string) {
	t.Helper()
	out := Run(context.Background(), c, Options{ID: id, Commit: rev, CommitSet: true}, strings.NewReader(""))
	if out.Error != nil {
		return task.Record{}, "", &out.Error.Code
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	return tasktest.Load(t, repo, id), b.String(), nil
}

func TestCommitBindsTheResolvedHead(t *testing.T) {
	repo, id := setup(t, task.Completed)
	tasktest.Register(t, repo, boss)
	gittest.Commit(t, repo)
	head := strings.TrimSpace(gittest.Git(t, repo, "rev-parse", "HEAD"))
	r, text, code := verifyCommit(t, repo, id, "HEAD")
	if code != nil || r.VerifiedCommit == nil || *r.VerifiedCommit != head || strings.Contains(text, "Warning") {
		t.Fatalf("%v %+v %q", code, r, text)
	}
	// A later verification without --commit replaces the bound commit.
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: id}, strings.NewReader(""))
	if r := tasktest.Load(t, repo, id); out.Error != nil || r.VerifiedCommit != nil {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

func TestCommitWarnsOnADirtyCheckout(t *testing.T) {
	repo, id := setup(t, task.Completed)
	tasktest.Register(t, repo, boss)
	gittest.Commit(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "scratch"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, text, code := verifyCommit(t, repo, id, "HEAD")
	if code != nil || r.VerifiedCommit == nil || !strings.Contains(text, "Warning: the checkout has uncommitted changes") {
		t.Fatalf("%v %+v %q", code, r, text)
	}
}

func TestCommitRejectsUnresolvableRevisions(t *testing.T) {
	repo, id := setup(t, task.Completed)
	tasktest.Register(t, repo, boss)
	gittest.Commit(t, repo)
	before := tasktest.Load(t, repo, id)
	for _, rev := range []string{"", "nosuch", "--all", "-h", "HEAD^{tree}"} {
		// The revision is refused before the caller is looked up.
		if _, _, code := verifyWith(t, tasktest.Client(t, repo, "w1:p1"), repo, id, rev); code == nil || *code != "invalid_input" {
			t.Fatalf("%q: %v", rev, code)
		}
	}
	if !reflect.DeepEqual(tasktest.Load(t, repo, id), before) {
		t.Fatal("record changed")
	}
}
