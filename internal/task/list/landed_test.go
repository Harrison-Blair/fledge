package list

import (
	"context"
	"slices"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestListFiltersLanded(t *testing.T) {
	repo := identitytest.Repository(t)
	landed := tasktest.Seed(t, repo, task.Record{Title: "landed", Status: task.Landed})
	tasktest.Seed(t, repo, task.Record{Title: "verified", Status: task.Verified})
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{Status: task.Landed})
	if out.Error != nil || !slices.Equal(ids(out.Result.(Result)), []string{landed}) {
		t.Fatalf("%+v %+v", out.Error, out.Result)
	}
}
