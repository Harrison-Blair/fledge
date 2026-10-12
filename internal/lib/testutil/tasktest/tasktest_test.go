package tasktest

import (
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

// Store opens the task store that Seed wrote.
func TestStoreReadsSeededTask(t *testing.T) {
	repo := identitytest.Repository(t)
	id := Seed(t, repo, task.Record{Title: "x"})
	r, err := task.Get(Store(t, repo), id)
	if err != nil || r.ID != id || r.Title != "x" {
		t.Fatalf("%+v %v", r, err)
	}
}
