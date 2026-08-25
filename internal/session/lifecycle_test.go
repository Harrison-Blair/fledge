package session

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/project"
)

func TestStartRejectsInsideHerderBeforeDiscovery(t *testing.T) {
	client := &fakeHerder{}
	err := Start(context.Background(), filepath.Join(t.TempDir(), "missing"), StartDependencies{
		Herder: client,
		Now:    time.Now,
		Getenv: func(name string) string {
			if name == "HERDR_ENV" {
				return "1"
			}
			return ""
		},
	})
	if err == nil || !strings.Contains(err.Error(), "inside Herder") {
		t.Fatalf("Start() error = %v, want inside-Herder rejection", err)
	}
	if client.listCalls != 0 || len(client.launches) != 0 {
		t.Fatalf("Start() called Herder after rejection: %#v", client)
	}
}

func TestStartCreatesFreshSessionFromNestedPath(t *testing.T) {
	root, nested := lifecycleProject(t, "My Project")
	writeLifecycleRecord(t, root, "fledge-old-00000001")
	client := &fakeHerder{sessions: []herdr.Session{
		{Name: "fledge-old-00000001", Running: false},
		{Name: "fledge-My-Project-00000002", Running: false},
		{Name: "unrelated", Running: true},
	}}
	now := time.Date(2026, 8, 24, 12, 13, 14, 0, time.UTC)

	err := Start(context.Background(), nested, StartDependencies{
		Herder:  client,
		Entropy: bytes.NewReader([]byte{0, 0, 0, 2, 0, 0, 0, 3}),
		Now:     func() time.Time { return now },
		Getenv:  emptyEnv,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	wantLaunches := []launchCall{{root: root, name: "fledge-My-Project-00000003"}}
	if !reflect.DeepEqual(client.launches, wantLaunches) {
		t.Fatalf("launches = %#v, want %#v", client.launches, wantLaunches)
	}
	records, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || !recordNamesContain(records, "fledge-old-00000001") || !recordNamesContain(records, "fledge-My-Project-00000003") {
		t.Fatalf("records = %#v, want old and fresh records", records)
	}
}

func TestStartAttachesToSoleRegisteredRunningSession(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "registered")
	client := &fakeHerder{sessions: []herdr.Session{
		{Name: "REGISTERED", Running: true},
		{Name: "registered", Running: true},
		{Name: "unrelated", Running: true},
	}}

	err := Start(context.Background(), root, startDeps(client))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	want := []launchCall{{root: root, name: "registered"}}
	if !reflect.DeepEqual(client.launches, want) {
		t.Fatalf("launches = %#v, want %#v", client.launches, want)
	}
	records, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("Start() created a record while attaching: %#v", records)
	}
}

func TestStartRejectsMultipleRegisteredRunningSessionsInSortedOrder(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "zeta")
	writeLifecycleRecord(t, root, "alpha")
	client := &fakeHerder{sessions: []herdr.Session{
		{Name: "zeta", Running: true},
		{Name: "alpha", Running: true},
	}}

	err := Start(context.Background(), root, startDeps(client))
	if err == nil || !strings.Contains(err.Error(), "alpha, zeta") {
		t.Fatalf("Start() error = %v, want sorted running names", err)
	}
	if len(client.launches) != 0 {
		t.Fatalf("Start() launches = %#v, want none", client.launches)
	}
}

func TestStartPropagatesListFailure(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	want := errors.New("list failed")
	client := &fakeHerder{listErr: want}

	err := Start(context.Background(), root, startDeps(client))
	if !errors.Is(err, want) {
		t.Fatalf("Start() error = %v, want wrapped %v", err, want)
	}
	if len(client.launches) != 0 {
		t.Fatalf("Start() launches = %#v, want none", client.launches)
	}
}

func TestStartRetainsFreshRecordAfterLaunchFailure(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	want := errors.New("launch failed")
	client := &fakeHerder{launchErr: want}

	err := Start(context.Background(), root, StartDependencies{
		Herder:  client,
		Entropy: bytes.NewReader([]byte{1, 2, 3, 4}),
		Now:     time.Now,
		Getenv:  emptyEnv,
	})
	if !errors.Is(err, want) {
		t.Fatalf("Start() error = %v, want wrapped %v", err, want)
	}
	records, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].HerdrSessionName != "fledge-project-01020304" {
		t.Fatalf("records after launch failure = %#v", records)
	}
}

func TestStartRejectsMalformedRecordsBeforeListing(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeRecord(t, root, "bad", `{}`)
	client := &fakeHerder{}

	err := Start(context.Background(), root, startDeps(client))
	if err == nil || !strings.Contains(err.Error(), "record") {
		t.Fatalf("Start() error = %v, want record error", err)
	}
	if client.listCalls != 0 {
		t.Fatalf("List() calls = %d, want 0", client.listCalls)
	}
}

func TestStopNoRunningSessionsReportsNoOp(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "stopped")
	client := &fakeHerder{sessions: []herdr.Session{{Name: "stopped", Running: false}}}
	var output bytes.Buffer

	err := Stop(context.Background(), root, StopDependencies{
		Herder: client,
		Output: &output,
		Getenv: emptyEnv,
	})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got := output.String(); !strings.Contains(got, root) || !strings.Contains(got, "No running") {
		t.Fatalf("Stop() output = %q, want project-root no-op", got)
	}
	if len(client.stops) != 0 {
		t.Fatalf("Stop() calls = %#v, want none", client.stops)
	}
}

func TestStopConfirmsSortedSnapshotAndStopsSelfLast(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	for _, name := range []string{"zeta", "alpha", "middle"} {
		writeLifecycleRecord(t, root, name)
	}
	client := &fakeHerder{sessions: []herdr.Session{
		{Name: "zeta", Running: true},
		{Name: "unrelated", Running: true},
		{Name: "middle", Running: true},
		{Name: "alpha", Running: true},
	}}
	confirmer := &fakeConfirmer{answer: true}

	err := Stop(context.Background(), root, StopDependencies{
		Herder:    client,
		Confirmer: confirmer,
		Output:    &bytes.Buffer{},
		Getenv: func(name string) string {
			if name == "HERDR_SESSION" {
				return "alpha"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if confirmer.root != root || !confirmer.selfStop {
		t.Fatalf("Confirm() root/self = %q/%v", confirmer.root, confirmer.selfStop)
	}
	if want := []string{"alpha", "middle", "zeta"}; !reflect.DeepEqual(confirmer.names, want) {
		t.Fatalf("Confirm() names = %#v, want %#v", confirmer.names, want)
	}
	if want := []string{"middle", "zeta", "alpha"}; !reflect.DeepEqual(client.stops, want) {
		t.Fatalf("Stop() order = %#v, want %#v", client.stops, want)
	}
	if client.listCalls != 1 {
		t.Fatalf("List() calls = %d, want one frozen snapshot", client.listCalls)
	}
	for _, name := range client.stops {
		if name == "unrelated" {
			t.Fatal("Stop() stopped unrelated Herder session")
		}
	}
}

func TestStopCancellationLeavesSnapshotUntouched(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "running")
	client := &fakeHerder{sessions: []herdr.Session{{Name: "running", Running: true}}}

	err := Stop(context.Background(), root, StopDependencies{
		Herder:    client,
		Confirmer: &fakeConfirmer{answer: false},
		Output:    &bytes.Buffer{},
		Getenv:    emptyEnv,
	})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(client.stops) != 0 {
		t.Fatalf("Stop() calls = %#v, want none", client.stops)
	}
}

func TestStopContinuesAndAggregatesFailuresByName(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	for _, name := range []string{"alpha", "beta", "gamma"} {
		writeLifecycleRecord(t, root, name)
	}
	first := errors.New("first failure")
	second := errors.New("second failure")
	client := &fakeHerder{
		sessions: []herdr.Session{
			{Name: "gamma", Running: true},
			{Name: "beta", Running: true},
			{Name: "alpha", Running: true},
		},
		stopErrors: map[string]error{"alpha": first, "gamma": second},
	}

	err := Stop(context.Background(), root, StopDependencies{
		Herder:    client,
		Confirmer: &fakeConfirmer{answer: true},
		Output:    &bytes.Buffer{},
		Getenv:    emptyEnv,
	})
	if !errors.Is(err, first) || !errors.Is(err, second) {
		t.Fatalf("Stop() error = %v, want both failures", err)
	}
	if got := err.Error(); !strings.Contains(got, `"alpha"`) || !strings.Contains(got, `"gamma"`) {
		t.Fatalf("Stop() error = %v, want failed names", err)
	}
	if want := []string{"alpha", "beta", "gamma"}; !reflect.DeepEqual(client.stops, want) {
		t.Fatalf("Stop() calls = %#v, want %#v", client.stops, want)
	}
}

func TestStopPropagatesListAndConfirmationFailures(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		root, _ := lifecycleProject(t, "project")
		want := errors.New("list failed")
		err := Stop(context.Background(), root, StopDependencies{
			Herder: &fakeHerder{listErr: want},
			Output: &bytes.Buffer{},
			Getenv: emptyEnv,
		})
		if !errors.Is(err, want) {
			t.Fatalf("Stop() error = %v, want wrapped %v", err, want)
		}
	})

	t.Run("confirmation", func(t *testing.T) {
		root, _ := lifecycleProject(t, "project")
		writeLifecycleRecord(t, root, "running")
		want := errors.New("confirmation failed")
		client := &fakeHerder{sessions: []herdr.Session{{Name: "running", Running: true}}}
		err := Stop(context.Background(), root, StopDependencies{
			Herder:    client,
			Confirmer: &fakeConfirmer{err: want},
			Output:    &bytes.Buffer{},
			Getenv:    emptyEnv,
		})
		if !errors.Is(err, want) {
			t.Fatalf("Stop() error = %v, want wrapped %v", err, want)
		}
		if len(client.stops) != 0 {
			t.Fatalf("Stop() calls = %#v, want none", client.stops)
		}
	})
}

type fakeHerder struct {
	sessions   []herdr.Session
	listErr    error
	launchErr  error
	stopErrors map[string]error
	listCalls  int
	launches   []launchCall
	stops      []string
}

type launchCall struct {
	root string
	name string
}

func (f *fakeHerder) List(context.Context) ([]herdr.Session, error) {
	f.listCalls++
	return append([]herdr.Session(nil), f.sessions...), f.listErr
}

func (f *fakeHerder) Launch(_ context.Context, root, name string) error {
	f.launches = append(f.launches, launchCall{root: root, name: name})
	return f.launchErr
}

func (f *fakeHerder) Stop(_ context.Context, name string) error {
	f.stops = append(f.stops, name)
	return f.stopErrors[name]
}

type fakeConfirmer struct {
	answer   bool
	err      error
	root     string
	names    []string
	selfStop bool
}

func (f *fakeConfirmer) Confirm(root string, names []string, selfStop bool) (bool, error) {
	f.root = root
	f.names = append([]string(nil), names...)
	f.selfStop = selfStop
	return f.answer, f.err
}

func startDeps(client Herder) StartDependencies {
	return StartDependencies{
		Herder:  client,
		Entropy: bytes.NewReader([]byte{1, 2, 3, 4}),
		Now:     time.Now,
		Getenv:  emptyEnv,
	}
}

func lifecycleProject(t *testing.T, base string) (string, string) {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, base)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical, err := project.Init(root)
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested", "directory")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	return canonical, nested
}

func writeLifecycleRecord(t *testing.T, root, name string) {
	t.Helper()
	writeRecord(t, root, name, `{"schema_version":1,"herdr_session_name":"`+name+`","created_at":"2026-08-24T14:15:16Z"}`)
}

func emptyEnv(string) string { return "" }

func recordNamesContain(records []Record, name string) bool {
	for _, record := range records {
		if record.HerdrSessionName == name {
			return true
		}
	}
	return false
}
