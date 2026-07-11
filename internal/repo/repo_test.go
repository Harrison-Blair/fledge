package repo

import (
	"path/filepath"
	"testing"
)

func TestRequirementsAndTasksDir(t *testing.T) {
	r := &Repo{Root: "/some/root"}

	wantReq := filepath.Join(r.FledgeDir(), "pluma", "plumage")
	if got := r.RequirementsDir(); got != wantReq {
		t.Errorf("RequirementsDir() = %q, want %q", got, wantReq)
	}

	wantTasks := filepath.Join(r.FledgeDir(), "pluma", "feathers")
	if got := r.TasksDir(); got != wantTasks {
		t.Errorf("TasksDir() = %q, want %q", got, wantTasks)
	}
}
