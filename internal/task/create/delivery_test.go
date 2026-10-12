package create

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// configure writes content as repo's .fledge/config.toml.
func configure(t *testing.T, repo, content string) {
	t.Helper()
	os.MkdirAll(filepath.Join(repo, ".fledge"), 0o755)
	if err := os.WriteFile(filepath.Join(repo, ".fledge", "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func created(t *testing.T, repo string, o Options) (task.Record, string) {
	t.Helper()
	o.Title, o.Body, o.BodySet = "t", "b", true
	out := Run(context.Background(), tasktest.Client(t, repo, ""), o, strings.NewReader(""))
	if out.Error != nil {
		return task.Record{}, out.Error.Code
	}
	return tasktest.Load(t, repo, out.Result.(task.Record).ID), ""
}

func TestCreateDeliveryModeDefaults(t *testing.T) {
	repo := identitytest.Repository(t)
	if r, code := created(t, repo, Options{}); code != "" || r.DeliveryMode != task.ModeNone || r.BaseBranch != nil {
		t.Fatalf("%s %+v", code, r)
	}
	configure(t, repo, "schema_version = 1\ndelivery_mode = \"pr\"\n")
	if r, code := created(t, repo, Options{}); code != "" || r.DeliveryMode != task.ModePR {
		t.Fatalf("config: %s %+v", code, r)
	}
	r, code := created(t, repo, Options{DeliveryMode: "local", DeliveryModeSet: true, BaseBranch: "release/1", BaseBranchSet: true})
	if code != "" || r.DeliveryMode != task.ModeLocal || r.BaseBranch == nil || *r.BaseBranch != "release/1" {
		t.Fatalf("flags: %s %+v", code, r)
	}
}

func TestCreateRejectsInvalidDeliveryOptions(t *testing.T) {
	repo := identitytest.Repository(t)
	for _, o := range []Options{
		{DeliveryMode: "", DeliveryModeSet: true},
		{DeliveryMode: "merge", DeliveryModeSet: true},
		{BaseBranch: "", BaseBranchSet: true},
		{BaseBranch: "-dev", BaseBranchSet: true},
		{BaseBranch: "a..b", BaseBranchSet: true},
		{BaseBranch: "a\nb", BaseBranchSet: true},
		{BaseBranch: "refs/remotes/origin/-foo", BaseBranchSet: true},
		{BaseBranch: "refs/heads/-foo", BaseBranchSet: true},
	} {
		if _, code := created(t, repo, o); code != "invalid_input" {
			t.Fatalf("%+v: %s", o, code)
		}
	}
	// An invalid configuration file fails the create unless --delivery-mode
	// makes it irrelevant.
	configure(t, repo, "schema_version = 1\ndelivery_mode = \"merge\"\n")
	if _, code := created(t, repo, Options{}); code == "" {
		t.Fatal("invalid config accepted")
	}
	if r, code := created(t, repo, Options{DeliveryMode: "none", DeliveryModeSet: true}); code != "" || r.DeliveryMode != task.ModeNone {
		t.Fatalf("%s %+v", code, r)
	}
}
