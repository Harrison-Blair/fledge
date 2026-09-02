package session

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"fledge/internal/herdr"
)

func TestRunningSessionReturnsSoleRegisteredRunningSession(t *testing.T) {
	root, nested := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "registered")
	writeLifecycleRecord(t, root, "stopped")
	client := &fakeHerder{sessions: []herdr.Session{
		{Name: "registered", Running: true},
		{Name: "stopped", Running: false},
		{Name: "unrelated", Running: true},
	}}

	name, err := RunningSession(context.Background(), nested, client.List)
	if err != nil {
		t.Fatalf("RunningSession() error = %v", err)
	}
	if name != "registered" {
		t.Fatalf("name = %q, want registered", name)
	}
}

func TestRunningSessionRejectsNoRunningSession(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "registered")
	client := &fakeHerder{sessions: []herdr.Session{
		{Name: "registered", Running: false},
		{Name: "unrelated", Running: true},
	}}

	name, err := RunningSession(context.Background(), root, client.List)
	if err == nil || !strings.Contains(err.Error(), "no running Fledge session for "+strconv.Quote(root)) {
		t.Fatalf("RunningSession() error = %v, want no-running-session error naming %q", err, root)
	}
	if name != "" {
		t.Fatalf("name = %q, want empty", name)
	}
}

func TestRunningSessionRejectsMultipleRunningSessionsInSortedOrder(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "zeta")
	writeLifecycleRecord(t, root, "alpha")
	client := &fakeHerder{sessions: []herdr.Session{
		{Name: "zeta", Running: true},
		{Name: "alpha", Running: true},
	}}

	_, err := RunningSession(context.Background(), root, client.List)
	if err == nil || !strings.Contains(err.Error(), "multiple registered sessions are running: alpha, zeta") {
		t.Fatalf("RunningSession() error = %v, want sorted running names", err)
	}
}

func TestRunningSessionPropagatesDiscoveryFailures(t *testing.T) {
	t.Run("outside a project", func(t *testing.T) {
		_, err := RunningSession(context.Background(), filepath.Join(t.TempDir(), "missing"), func(context.Context) ([]herdr.Session, error) {
			t.Fatal("listed Herder sessions outside a project")
			return nil, nil
		})
		if err == nil {
			t.Fatal("RunningSession() error = nil, want project discovery failure")
		}
	})

	t.Run("list failure", func(t *testing.T) {
		root, _ := lifecycleProject(t, "project")
		want := errors.New("list failed")

		_, err := RunningSession(context.Background(), root, (&fakeHerder{listErr: want}).List)
		if !errors.Is(err, want) {
			t.Fatalf("RunningSession() error = %v, want %v", err, want)
		}
	})
}
