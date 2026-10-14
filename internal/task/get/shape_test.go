package get

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestGetShowsShapeAndBranch(t *testing.T) {
	repo := identitytest.Repository(t)
	id := tasktest.Seed(t, repo, task.Record{Title: "T", Brief: "b", Status: task.Created, CreatedAt: "2026-01-01T00:00:00Z",
		Shape: task.ShapeStack, Branch: tasktest.Ptr("stack/01-a")})
	var b bytes.Buffer
	Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id}).Write(&b, false, Render)
	for _, want := range []string{"\nshape: stack\n", "\nbranch: stack/01-a\n"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("lacks %q: %s", want, b.String())
		}
	}
}
