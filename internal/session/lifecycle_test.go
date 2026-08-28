package session

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/project"
	"fledge/internal/session/bootstrap"
	"fledge/internal/session/lock"
	"fledge/internal/session/record"
	"fledge/internal/session/sessiontest"
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

func TestInitialNameLimitValidatesDefaultSessionDirectory(t *testing.T) {
	valid := herdr.Session{Default: true, SessionDir: "/tmp/herdr"}
	tests := []struct {
		name     string
		sessions []herdr.Session
		wantErr  string
	}{
		{name: "no default", sessions: nil, wantErr: "exactly one default"},
		{name: "two defaults", sessions: []herdr.Session{valid, valid}, wantErr: "exactly one default"},
		{name: "empty", sessions: []herdr.Session{{Default: true}}, wantErr: "invalid session_dir"},
		{name: "relative", sessions: []herdr.Session{{Default: true, SessionDir: "relative"}}, wantErr: "invalid session_dir"},
		{name: "nul", sessions: []herdr.Session{{Default: true, SessionDir: "/tmp/\x00herdr"}}, wantErr: "invalid session_dir"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := initialNameLimit(test.sessions); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("initialNameLimit() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestInitialNameLimitPreservesRawSessionDirectoryAndSocketCapacity(t *testing.T) {
	dir := "/tmp/ユニコード/./double//"
	limit, err := initialNameLimit([]herdr.Session{{Default: true, SessionDir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	want := min(record.MaxSessionLength, 103-len(dir+"sessions")-2-len("herdr.sock"), 103-len(dir+"sessions")-2-len("herdr-client.sock"))
	if limit != want {
		t.Fatalf("initialNameLimit() = %d, want %d using the raw session directory", limit, want)
	}
	for _, socket := range []string{"herdr.sock", "herdr-client.sock"} {
		path := dir + "sessions/" + strings.Repeat("a", limit) + "/" + socket
		if len(path) > 103 {
			t.Fatalf("%s path length = %d, want <= 103", socket, len(path))
		}
	}
}

func TestInitialNameLimitRejectsSixteenAndAcceptsSeventeen(t *testing.T) {
	tooLong := "/" + strings.Repeat("a", 59-1)
	if _, err := initialNameLimit([]herdr.Session{{Default: true, SessionDir: tooLong}}); err == nil || !strings.Contains(err.Error(), "no usable") {
		t.Fatalf("initialNameLimit() for 16-byte capacity error = %v, want rejection", err)
	}
	accepted := "/" + strings.Repeat("a", 58-1)
	limit, err := initialNameLimit([]herdr.Session{{Default: true, SessionDir: accepted}})
	if err != nil {
		t.Fatalf("initialNameLimit() for 17-byte capacity error = %v", err)
	}
	if limit != record.MinSessionLength {
		t.Fatalf("initialNameLimit() for 17-byte capacity = %d, want %d", limit, record.MinSessionLength)
	}
}

func TestStartRejectsInvalidNameLimitBeforeChoiceOrFilesystemEffects(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	chooser := &fakeChooser{}
	entropy := &sessiontest.CountingReader{}
	client := &fakeHerder{sessions: []herdr.Session{{Default: true, SessionDir: "/" + strings.Repeat("a", 58)}}}
	deps := StartDependencies{
		Herder:  client,
		Entropy: entropy,
		Now:     time.Now,
		Getenv:  emptyEnv,
		Chooser: chooser,
		acquireLock: func(context.Context, string) (func() error, error) {
			return func() error { return nil }, nil
		},
	}
	err := Start(context.Background(), root, deps)
	if err == nil || !strings.Contains(err.Error(), "no usable session-name capacity") {
		t.Fatalf("Start() error = %v, want invalid-limit error", err)
	}
	if chooser.calls != 0 || entropy.Reads != 0 {
		t.Fatalf("Start() chooser/entropy reads = %d/%d, want 0/0", chooser.calls, entropy.Reads)
	}
	if _, err := os.Lstat(filepath.Join(root, ".fledge", "sessions")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sessions directory after invalid limit error = %v, want absent", err)
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

	deps, _ := freshStartDeps(client, &fakeChooser{}, sessiontest.StartedBootstrapper(), &bytes.Buffer{})
	deps.Entropy = bytes.NewReader([]byte{0, 0, 0, 2, 0, 0, 0, 3})
	deps.Now = func() time.Time { return now }
	err := Start(context.Background(), nested, deps)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	wantLaunches := []launchCall{{root: root, name: "fledge-My-Project-00000003"}}
	if !reflect.DeepEqual(client.launches, wantLaunches) {
		t.Fatalf("launches = %#v, want %#v", client.launches, wantLaunches)
	}
	records, err := record.Load(root)
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
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("Start() created a record while attaching: %#v", records)
	}
}

func TestStartClaimsAndReleasesBeforeHistoricalAttach(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "registered")
	var released bool
	client := &fakeHerder{sessions: []herdr.Session{{Name: "registered", Running: true}}}
	client.onLaunch = func(call launchCall) {
		if !released {
			t.Errorf("Launch(%q) ran before project lock release", call.name)
		}
	}
	deps := startDeps(client)
	deps.acquireLock = func(context.Context, string) (func() error, error) {
		return func() error {
			released = true
			return nil
		}, nil
	}

	if err := Start(context.Background(), root, deps); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !records[0].Claimed {
		t.Fatalf("records after historical attach = %#v, want claimed record", records)
	}
}

func TestStartReleaseFailureAbortsHistoricalAttach(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "registered")
	releaseErr := errors.New("release failed")
	client := &fakeHerder{sessions: []herdr.Session{{Name: "registered", Running: true}}}
	deps := startDeps(client)
	deps.acquireLock = func(context.Context, string) (func() error, error) {
		return func() error { return releaseErr }, nil
	}

	err := Start(context.Background(), root, deps)
	if !errors.Is(err, releaseErr) {
		t.Fatalf("Start() error = %v, want release failure", err)
	}
	if len(client.launches) != 0 {
		t.Fatalf("Launches = %#v, want none after failed release", client.launches)
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

	deps, _ := freshStartDeps(client, &fakeChooser{}, sessiontest.StartedBootstrapper(), &bytes.Buffer{})
	err := Start(context.Background(), root, deps)
	if !errors.Is(err, want) {
		t.Fatalf("Start() error = %v, want wrapped %v", err, want)
	}
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].HerdrSessionName != "fledge-project-01020304" {
		t.Fatalf("records after launch failure = %#v", records)
	}
}

func TestConcurrentStartsSerializeChooserClaimAndName(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	ctx, cancel := context.WithCancel(testContext(t))
	chooser := &gatedChooser{choice: AgentChoice{}, entered: make(chan struct{}), allow: make(chan struct{})}
	releaseChooser := closeOnCleanup(t, chooser.allow)
	launchRelease := make(chan struct{})
	releaseLaunch := closeOnCleanup(t, launchRelease)
	launchStarted := make(chan launchCall, 2)
	launchErr := errors.New("launch failed")
	client := &fakeHerder{
		launchWait: launchRelease,
		launchErr:  launchErr,
		onLaunchStart: func(call launchCall) {
			launchStarted <- call
		},
	}
	deps, _ := freshStartDeps(client, chooser, sessiontest.StartedBootstrapper(), &bytes.Buffer{})
	entropy := &countedReader{Reader: bytes.NewReader([]byte{1, 2, 3, 4})}
	deps.Entropy = entropy
	secondAcquire := make(chan struct{})
	var acquireMu sync.Mutex
	acquireCalls := 0
	deps.acquireLock = func(ctx context.Context, fledgeDir string) (func() error, error) {
		acquireMu.Lock()
		acquireCalls++
		call := acquireCalls
		acquireMu.Unlock()
		if call == 2 {
			close(secondAcquire)
		}
		return lock.Acquire(ctx, fledgeDir)
	}

	errs := make(chan error, 2)
	workerErrs := make(chan error, 2)
	pendingResults := 0
	t.Cleanup(func() {
		releaseChooser()
		releaseLaunch()
		cancel()
		drainTestWorkers(t, errs, &pendingResults)
	})
	runStart := func() {
		err := Start(ctx, root, deps)
		workerErrs <- err
		errs <- err
	}
	pendingResults++
	go runStart()
	awaitTestEvent(t, ctx, chooser.entered, workerErrs)
	pendingResults++
	go runStart()
	releaseChooser()
	first := awaitTestEvent(t, ctx, launchStarted, workerErrs)
	awaitTestEvent(t, ctx, secondAcquire, workerErrs)
	select {
	case call := <-launchStarted:
		t.Fatalf("second Launch(%q) began before the first released the project lock", call.name)
	default:
	}
	releaseLaunch()
	second := awaitTestEvent(t, ctx, launchStarted, nil)
	for range 2 {
		if err := awaitTestEvent(t, ctx, errs, nil); !errors.Is(err, launchErr) {
			t.Fatalf("Start() error = %v, want %v", err, launchErr)
		}
		pendingResults--
	}
	if chooser.Calls() != 1 {
		t.Fatalf("Choose() calls = %d, want one", chooser.Calls())
	}
	if entropy.Reads() != 1 {
		t.Fatalf("entropy reads = %d, want one name generation", entropy.Reads())
	}
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !records[0].Claimed || records[0].PendingChoice == nil {
		t.Fatalf("records = %#v, want one claimed pending record", records)
	}
	if first.name != records[0].HerdrSessionName || second.name != first.name {
		t.Fatalf("launch names = %q, %q; claimed name = %q", first.name, second.name, records[0].HerdrSessionName)
	}
}

func TestStableClaimAfterLaunchFailure(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	launchErr := errors.New("launch failed")
	client := &fakeHerder{launchErr: launchErr}
	chooser := &fakeChooser{choice: AgentChoice{}}
	deps, _ := freshStartDeps(client, chooser, sessiontest.StartedBootstrapper(), &bytes.Buffer{})
	entropy := &countedReader{Reader: bytes.NewReader([]byte{1, 2, 3, 4})}
	deps.Entropy = entropy

	if err := Start(context.Background(), root, deps); !errors.Is(err, launchErr) {
		t.Fatalf("first Start() error = %v, want %v", err, launchErr)
	}
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !records[0].Claimed || records[0].PendingChoice == nil {
		t.Fatalf("records after failed launch = %#v, want claimed pending record", records)
	}
	name := records[0].HerdrSessionName
	if err := Start(context.Background(), root, deps); !errors.Is(err, launchErr) {
		t.Fatalf("second Start() error = %v, want %v", err, launchErr)
	}
	if chooser.calls != 1 {
		t.Fatalf("Choose() calls = %d, want no second choice", chooser.calls)
	}
	if entropy.Reads() != 1 {
		t.Fatalf("entropy reads = %d, want no second name generation", entropy.Reads())
	}
	if launches := client.Launches(); len(launches) != 2 || launches[0].name != name || launches[1].name != name {
		t.Fatalf("launches = %#v, want two attempts for %q", launches, name)
	}
	if records, err := record.Load(root); err != nil || len(records) != 1 || records[0].HerdrSessionName != name {
		t.Fatalf("records after retry = %#v, %v; want one stable record", records, err)
	}
}

func TestStableClaimAfterDelayedLaunch(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	ctx, cancel := context.WithCancel(testContext(t))
	launchErr := errors.New("launch failed")
	launchRelease := make(chan struct{})
	releaseLaunch := closeOnCleanup(t, launchRelease)
	launchStarted := make(chan launchCall, 2)
	client := &fakeHerder{
		launchWait: launchRelease,
		launchErr:  launchErr,
		onLaunchStart: func(call launchCall) {
			launchStarted <- call
		},
	}
	lock := newSerialTestLock()
	deps, _ := freshStartDeps(client, &fakeChooser{choice: AgentChoice{}}, sessiontest.StartedBootstrapper(), &bytes.Buffer{})
	deps.acquireLock = lock.acquire

	errs := make(chan error, 2)
	workerErrs := make(chan error, 2)
	pendingResults := 0
	t.Cleanup(func() {
		releaseLaunch()
		cancel()
		drainTestWorkers(t, errs, &pendingResults)
	})
	runStart := func() {
		err := Start(ctx, root, deps)
		workerErrs <- err
		errs <- err
	}
	pendingResults++
	go runStart()
	first := awaitTestEvent(t, ctx, launchStarted, workerErrs)
	pendingResults++
	go runStart()
	awaitTestEvent(t, ctx, lock.secondAttempt, workerErrs)
	select {
	case call := <-launchStarted:
		t.Fatalf("second Launch(%q) began before the first released the lock", call.name)
	default:
	}
	releaseLaunch()
	second := awaitTestEvent(t, ctx, launchStarted, nil)
	for range 2 {
		if err := awaitTestEvent(t, ctx, errs, nil); !errors.Is(err, launchErr) {
			t.Fatalf("Start() error = %v, want %v", err, launchErr)
		}
		pendingResults--
	}
	if first.name != second.name {
		t.Fatalf("delayed launch names = %q, %q; want a stable claim", first.name, second.name)
	}
}

func TestStableClaimAfterSessionDeletion(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "gone")
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Claim(records[0]); err != nil {
		t.Fatal(err)
	}
	launchErr := errors.New("session is gone")
	client := &fakeHerder{launchErr: launchErr}

	err = Start(context.Background(), root, startDeps(client))
	if !errors.Is(err, launchErr) {
		t.Fatalf("Start() error = %v, want %v", err, launchErr)
	}
	if launches := client.Launches(); len(launches) != 1 || launches[0].name != "gone" {
		t.Fatalf("launches after external deletion = %#v, want the local claim reused", launches)
	}
	if records, err := record.Load(root); err != nil || len(records) != 1 || !records[0].Claimed {
		t.Fatalf("records after external deletion = %#v, %v; want retained claim", records, err)
	}
}

func TestStartNewDiscardsStoppedClaimAndCreatesFreshSession(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "fledge-old-00000001")
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Claim(records[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(records[0].Path, "pending.json"), []byte(`{"schema_version":1,"harness":"claude","model":"opus"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerder{sessions: []herdr.Session{{Name: "fledge-old-00000001", Running: false}}}
	chooser := &fakeChooser{choice: AgentChoice{Harness: "claude", Model: "opus"}}
	server := sessiontest.StartedBootstrapper()
	released := make(chan struct{})
	release := closeOnCleanup(t, released)
	server.OnStart = func(context.Context, int) { release() }
	client.launchWait = released
	var diagnostics bytes.Buffer

	deps, scoped := freshStartDeps(client, chooser, server, &diagnostics)
	deps.New = true
	if err := Start(context.Background(), root, deps); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if chooser.calls != 1 {
		t.Fatalf("Choose() calls = %d, want one", chooser.calls)
	}
	newRecords, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(newRecords) != 2 {
		t.Fatalf("records = %#v, want 2", newRecords)
	}
	old := recordByName(newRecords, "fledge-old-00000001")
	if old.Claimed || old.PendingChoice != nil {
		t.Fatalf("old record = %#v, want unclaimed with no pending choice", old)
	}
	name := "fledge-project-01020304"
	fresh := recordByName(newRecords, name)
	if !fresh.Claimed {
		t.Fatalf("fresh record = %#v, want claimed", fresh)
	}
	if want := []launchCall{{root: root, name: name}}; !reflect.DeepEqual(client.Launches(), want) {
		t.Fatalf("launches = %#v, want %#v", client.Launches(), want)
	}
	if want := []string{name}; !reflect.DeepEqual(*scoped, want) {
		t.Fatalf("bootstrapped sessions = %#v, want %#v", *scoped, want)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("diagnostics = %q, want empty", diagnostics.String())
	}
}

func TestStartNewRejectsRunningRegisteredSession(t *testing.T) {
	for _, test := range []struct {
		name  string
		claim bool
	}{
		{name: "claimed", claim: true},
		{name: "unclaimed", claim: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, _ := lifecycleProject(t, "project")
			writeLifecycleRecord(t, root, "registered")
			if test.claim {
				records, err := record.Load(root)
				if err != nil {
					t.Fatal(err)
				}
				if err := record.Claim(records[0]); err != nil {
					t.Fatal(err)
				}
			}
			client := &fakeHerder{sessions: []herdr.Session{{Name: "registered", Running: true}}}
			chooser := &fakeChooser{}

			deps, _ := freshStartDeps(client, chooser, sessiontest.StartedBootstrapper(), &bytes.Buffer{})
			deps.New = true
			var released bool
			deps.acquireLock = func(context.Context, string) (func() error, error) {
				return func() error {
					released = true
					return nil
				}, nil
			}

			err := Start(context.Background(), root, deps)
			if err == nil || !strings.Contains(err.Error(), "fledge stop") {
				t.Fatalf("Start() error = %v, want a %q hint", err, "fledge stop")
			}
			if !released {
				t.Fatal("project lock was not released")
			}
			if chooser.calls != 0 {
				t.Fatalf("Choose() calls = %d, want 0", chooser.calls)
			}
			if len(client.Launches()) != 0 {
				t.Fatalf("launches = %#v, want none", client.Launches())
			}
			if test.claim {
				records, err := record.Load(root)
				if err != nil {
					t.Fatal(err)
				}
				if !records[0].Claimed {
					t.Fatalf("records after rejected --new = %#v, want claim retained", records)
				}
				if _, err := os.Stat(filepath.Join(records[0].Path, "claim.json")); err != nil {
					t.Fatalf("claim.json after rejected --new: %v", err)
				}
			}
		})
	}
}

func TestStartNewWithoutClaimUsesPickerPath(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "fledge-old-00000001")
	client := &fakeHerder{sessions: []herdr.Session{{Name: "fledge-old-00000001", Running: false}}}
	chooser := &fakeChooser{choice: AgentChoice{Harness: "claude", Model: "opus"}}
	server := sessiontest.StartedBootstrapper()
	released := make(chan struct{})
	release := closeOnCleanup(t, released)
	server.OnStart = func(context.Context, int) { release() }
	client.launchWait = released
	var diagnostics bytes.Buffer

	deps, scoped := freshStartDeps(client, chooser, server, &diagnostics)
	deps.New = true
	if err := Start(context.Background(), root, deps); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if chooser.calls != 1 {
		t.Fatalf("Choose() calls = %d, want one", chooser.calls)
	}
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %#v, want 2", records)
	}
	name := "fledge-project-01020304"
	if want := []string{name}; !reflect.DeepEqual(*scoped, want) {
		t.Fatalf("bootstrapped sessions = %#v, want %#v", *scoped, want)
	}
}

func TestStartRejectsMalformedRecordsBeforeListing(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	sessiontest.WriteRecord(t, root, "bad", `{}`)
	client := &fakeHerder{}

	err := Start(context.Background(), root, startDeps(client))
	if err == nil || !strings.Contains(err.Error(), "record") {
		t.Fatalf("Start() error = %v, want record error", err)
	}
	if client.listCalls != 0 {
		t.Fatalf("List() calls = %d, want 0", client.listCalls)
	}
}

func TestStartRejectsInvalidStopIntentBeforeClaimOrLaunch(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "managed")
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	rec := recordByName(records, "managed")
	if err := os.WriteFile(filepath.Join(rec.Path, record.StopIntentFileName), []byte(`{"schema_version":1,"intent_id":"invalid"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerder{sessions: []herdr.Session{{Name: "managed", Running: true}}}
	err = Start(context.Background(), root, startDeps(client))
	if err == nil || !strings.Contains(err.Error(), "read stop intent") {
		t.Fatalf("Start() error = %v, want invalid stop intent", err)
	}
	if len(client.launches) != 0 {
		t.Fatalf("Start() launches = %q, want none", client.launches)
	}
	if _, err := os.Lstat(filepath.Join(rec.Path, "claim.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("claim after rejected intent = %v, want absent", err)
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
			switch name {
			case "HERDR_ENV":
				return "1"
			case "HERDR_SESSION":
				return "alpha"
			case "HERDR_PANE_ID":
				return "pane-alpha"
			}
			return ""
		},
		Scoped: func(string) PaneResolver {
			return lifecyclePaneResolver{pane: herdr.Pane{ID: "pane-alpha", WorkspaceID: "workspace", TabID: "tab"}}
		},
		Entropy: bytes.NewReader(make([]byte, 16)),
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

func TestStopRejectsManagedCrossProjectIdentityBeforeConfirmation(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "session-a")
	client := &fakeHerder{sessions: []herdr.Session{{Name: "session-a", Running: true}, {Name: "session-b", Running: true}}}
	confirmer := &fakeConfirmer{answer: true}
	env := map[string]string{"HERDR_ENV": "1", "HERDR_SESSION": "session-b", "HERDR_PANE_ID": "pane-b"}
	err := Stop(context.Background(), root, StopDependencies{
		Herder:    client,
		Confirmer: confirmer,
		Output:    &bytes.Buffer{},
		Getenv:    func(name string) string { return env[name] },
		Scoped: func(string) PaneResolver {
			t.Fatal("resolved a pane for a cross-project session")
			return nil
		},
		Entropy: bytes.NewReader(make([]byte, 16)),
	})
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("Stop() error = %v, want cross-project rejection", err)
	}
	if confirmer.names != nil || len(client.stops) != 0 {
		t.Fatalf("Stop() confirmed or mutated after rejection: names=%q stops=%q", confirmer.names, client.stops)
	}
}

func TestStopRejectsInvalidIntentBeforeStopping(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "managed")
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recordByName(records, "managed").Path, record.StopIntentFileName), []byte(`{"schema_version":1,"intent_id":"bad"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerder{sessions: []herdr.Session{{Name: "managed", Running: true}}}
	err = Stop(context.Background(), root, StopDependencies{
		Herder:    client,
		Confirmer: &fakeConfirmer{answer: true},
		Output:    &bytes.Buffer{},
		Getenv:    emptyEnv,
		Entropy:   bytes.NewReader(make([]byte, 16)),
	})
	if err == nil || !strings.Contains(err.Error(), "read stop intent") {
		t.Fatalf("Stop() error = %v, want invalid intent error", err)
	}
	if len(client.stops) != 0 {
		t.Fatalf("Stop() calls = %q, want none", client.stops)
	}
}

func TestOverlappingStopsSerializeMarkerAndHerderMutation(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "managed")
	client := &fakeHerder{sessions: []herdr.Session{{Name: "managed", Running: true}}}
	firstEntered := make(chan struct{})
	allowFirst := make(chan struct{})
	var stopCalls int
	client.onStop = func(string) {
		stopCalls++
		if stopCalls == 1 {
			close(firstEntered)
			<-allowFirst
		}
	}
	lock := newSerialTestLock()
	run := func(fill byte) error {
		return Stop(context.Background(), root, StopDependencies{
			Herder:      client,
			Confirmer:   &fakeConfirmer{answer: true},
			Output:      &bytes.Buffer{},
			Getenv:      emptyEnv,
			Entropy:     bytes.NewReader(bytes.Repeat([]byte{fill}, 16)),
			acquireLock: lock.acquire,
		})
	}
	firstDone := make(chan error, 1)
	go func() { firstDone <- run(1) }()
	waitClosed(t, firstEntered, "first stop did not reach Herder")
	secondDone := make(chan error, 1)
	go func() { secondDone <- run(2) }()
	waitClosed(t, lock.secondAttempt, "second stop did not wait for project lock")
	client.mu.Lock()
	stopsBeforeRelease := len(client.stops)
	client.mu.Unlock()
	if stopsBeforeRelease != 1 {
		t.Fatalf("Herder stops before first release = %d, want 1", stopsBeforeRelease)
	}
	close(allowFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := record.ReadStopIntent(recordByName(records, "managed"))
	want := strings.Repeat("02", 16)
	if err != nil || !intent.Exists || intent.ID != want {
		t.Fatalf("final stop intent = %#v, %v; want %q", intent, err, want)
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
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	oldAlpha := strings.Repeat("a", 32)
	if err := record.WriteStopIntent(recordByName(records, "alpha"), oldAlpha); err != nil {
		t.Fatal(err)
	}

	err = Stop(context.Background(), root, StopDependencies{
		Herder:    client,
		Confirmer: &fakeConfirmer{answer: true},
		Output:    &bytes.Buffer{},
		Getenv:    emptyEnv,
		Entropy:   bytes.NewReader(make([]byte, 16)),
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
	alpha, err := record.ReadStopIntent(recordByName(records, "alpha"))
	if err != nil || !alpha.Exists || alpha.ID != oldAlpha {
		t.Fatalf("alpha stop intent = %#v, %v; want restored %q", alpha, err, oldAlpha)
	}
	beta, err := record.ReadStopIntent(recordByName(records, "beta"))
	if err != nil || !beta.Exists || beta.ID != strings.Repeat("0", 32) {
		t.Fatalf("beta stop intent = %#v, %v; want invocation intent", beta, err)
	}
	gamma, err := record.ReadStopIntent(recordByName(records, "gamma"))
	if err != nil || gamma.Exists {
		t.Fatalf("gamma stop intent = %#v, %v; want restored absence", gamma, err)
	}
}

func TestStartClassifiesOnlyExplicitIntentTransitionsAsCleanShutdown(t *testing.T) {
	launchFailure := errors.New("exit status 1")
	tests := []struct {
		name           string
		before         string
		after          string
		runningAfter   bool
		cancelOnLaunch bool
		listAfterErr   error
		wantNil        bool
	}{
		{name: "explicit stop", after: strings.Repeat("1", 32), runningAfter: false, wantNil: true},
		{name: "historical intent unchanged", before: strings.Repeat("1", 32), after: strings.Repeat("1", 32), runningAfter: false},
		{name: "external stop without intent", runningAfter: false},
		{name: "crash while still running", runningAfter: true},
		{name: "changed intent but still running", after: strings.Repeat("2", 32), runningAfter: true},
		{name: "canceled caller", after: strings.Repeat("3", 32), runningAfter: false, cancelOnLaunch: true},
		{name: "post-stop list failure", after: strings.Repeat("4", 32), runningAfter: false, listAfterErr: errors.New("list failed")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, _ := lifecycleProject(t, "project")
			writeLifecycleRecord(t, root, "managed")
			records, err := record.Load(root)
			if err != nil {
				t.Fatal(err)
			}
			rec := recordByName(records, "managed")
			if test.before != "" {
				if err := record.WriteStopIntent(rec, test.before); err != nil {
					t.Fatal(err)
				}
			}
			client := &fakeHerder{sessions: []herdr.Session{{Name: "managed", Running: true}}, launchErr: launchFailure}
			if test.listAfterErr != nil {
				client.listResults = []listResult{
					{sessions: []herdr.Session{{Name: "managed", Running: true}}},
					{err: test.listAfterErr},
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client.onLaunch = func(launchCall) {
				if test.after != "" && test.after != test.before {
					if err := record.WriteStopIntent(rec, test.after); err != nil {
						t.Error(err)
					}
				}
				client.mu.Lock()
				client.sessions = []herdr.Session{{Name: "managed", Running: test.runningAfter}}
				client.mu.Unlock()
				if test.cancelOnLaunch {
					cancel()
				}
			}
			err = Start(ctx, root, startDeps(client))
			if test.wantNil {
				if err != nil {
					t.Fatalf("Start() error = %v, want clean shutdown", err)
				}
				return
			}
			if !errors.Is(err, launchFailure) {
				t.Fatalf("Start() error = %v, want launch failure", err)
			}
			if test.listAfterErr != nil && !errors.Is(err, test.listAfterErr) {
				t.Fatalf("Start() error = %v, want list failure", err)
			}
		})
	}
}

func TestExplicitStopMakesForegroundStartExitCleanly(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "managed")
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	rec := recordByName(records, "managed")
	launchStarted := make(chan struct{})
	serverStopped := make(chan struct{})
	client := &fakeHerder{
		sessions:      []herdr.Session{{Name: "managed", Running: true}},
		launchWait:    serverStopped,
		launchErr:     errors.New("exit status 1"),
		onLaunchStart: func(launchCall) { close(launchStarted) },
	}
	client.onStop = func(name string) {
		intent, markerErr := record.ReadStopIntent(rec)
		if markerErr != nil || !intent.Exists || intent.ID != strings.Repeat("06", 16) {
			t.Errorf("intent at Herder stop = %#v, %v", intent, markerErr)
		}
		client.mu.Lock()
		client.sessions = []herdr.Session{{Name: name, Running: false}}
		client.mu.Unlock()
		close(serverStopped)
	}
	startDone := make(chan error, 1)
	go func() { startDone <- Start(context.Background(), root, startDeps(client)) }()
	waitClosed(t, launchStarted, "foreground start did not reach Herder")
	if err := Stop(context.Background(), root, StopDependencies{
		Herder:    client,
		Confirmer: &fakeConfirmer{answer: true},
		Output:    &bytes.Buffer{},
		Getenv:    emptyEnv,
		Entropy:   bytes.NewReader(bytes.Repeat([]byte{6}, 16)),
	}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("Start() error = %v, want clean intentional shutdown", err)
		}
	case <-time.After(testAsyncTimeout):
		t.Fatal("foreground Start() did not return")
	}
}

func TestStartClaimedReleasesOriginalLockBeforeIntentInspection(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	recordPath := filepath.Join(root, ".fledge", "sessions", "managed")
	if err := os.MkdirAll(recordPath, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := record.Record{HerdrSessionName: "managed", Path: recordPath}
	launchFailure := errors.New("exit status 1")
	client := &fakeHerder{sessions: []herdr.Session{{Name: "managed", Running: true}}, launchErr: launchFailure}
	originalReleased := false
	client.onLaunch = func(launchCall) {
		if err := record.WriteStopIntent(rec, strings.Repeat("5", 32)); err != nil {
			t.Error(err)
		}
		client.mu.Lock()
		client.sessions = []herdr.Session{{Name: "managed", Running: false}}
		client.mu.Unlock()
	}
	deps := StartDependencies{
		Herder: client,
		acquireLock: func(context.Context, string) (func() error, error) {
			if !originalReleased {
				t.Fatal("intent inspection reacquired before original lock release")
			}
			return func() error { return nil }, nil
		},
	}
	err := startClaimed(context.Background(), root, rec, deps, cachedRelease(func() error {
		originalReleased = true
		return nil
	}))
	if err != nil {
		t.Fatalf("startClaimed() error = %v, want intentional stop suppressed", err)
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
	mu sync.Mutex

	sessions        []herdr.Session
	listResults     []listResult
	listWait        <-chan struct{}
	waitForListCall int
	launchWait      <-chan struct{}
	listErr         error
	launchErr       error
	stopErrors      map[string]error
	onList          func(int)
	onLaunchStart   func(launchCall)
	onLaunch        func(launchCall)
	onStop          func(string)
	listCalls       int
	launches        []launchCall
	stops           []string
}

type listResult struct {
	sessions []herdr.Session
	err      error
}

type launchCall struct {
	root string
	name string
}

const testAsyncTimeout = 5 * time.Second

func waitClosed(t *testing.T, channel <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(testAsyncTimeout):
		t.Fatal(failure)
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testAsyncTimeout)
	t.Cleanup(cancel)
	return ctx
}

// awaitTestEvent bounds asynchronous test coordination and surfaces a worker
// failure that arrives before the expected progress event.
func awaitTestEvent[T any](t *testing.T, ctx context.Context, event <-chan T, workerErr <-chan error) T {
	t.Helper()
	select {
	case value := <-event:
		return value
	case err := <-workerErr:
		t.Fatalf("worker returned before expected progress: %v", err)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for test progress: %v", ctx.Err())
	}
	var zero T
	return zero
}

func closeOnCleanup[T any](t *testing.T, ch chan T) func() {
	t.Helper()
	var once sync.Once
	closeChannel := func() { once.Do(func() { close(ch) }) }
	t.Cleanup(closeChannel)
	return closeChannel
}

func drainTestWorkers(t *testing.T, results <-chan error, pending *int) {
	t.Helper()
	timer := time.NewTimer(testAsyncTimeout)
	defer timer.Stop()
	for *pending > 0 {
		select {
		case <-results:
			*pending = *pending - 1
		case <-timer.C:
			t.Errorf("timed out draining %d asynchronous worker result(s)", *pending)
			return
		}
	}
}

func (f *fakeHerder) List(ctx context.Context) ([]herdr.Session, error) {
	f.mu.Lock()
	f.listCalls++
	call := f.listCalls
	sessions := f.sessions
	err := f.listErr
	if len(f.listResults) != 0 {
		result := f.listResults[min(call, len(f.listResults))-1]
		sessions = result.sessions
		err = result.err
	}
	sessions = append([]herdr.Session(nil), sessions...)
	notify := f.onList
	wait := f.listWait
	waitForCall := f.waitForListCall
	f.mu.Unlock()
	if wait != nil && call == waitForCall {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
		}
	}

	for _, session := range sessions {
		if session.Default {
			if notify != nil {
				notify(call)
			}
			return sessions, err
		}
	}
	if notify != nil {
		notify(call)
	}
	return append(sessions, herdr.Session{
		Name:       "default",
		Default:    true,
		SocketPath: "/tmp/herdr/herdr.sock",
		SessionDir: "/tmp/herdr",
	}), err
}

func (f *fakeHerder) Launch(ctx context.Context, root, name string) error {
	call := launchCall{root: root, name: name}
	f.mu.Lock()
	started := f.onLaunchStart
	f.mu.Unlock()
	if started != nil {
		started(call)
	}
	if f.launchWait != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.launchWait:
		}
	}
	f.mu.Lock()
	f.launches = append(f.launches, call)
	err := f.launchErr
	notify := f.onLaunch
	f.mu.Unlock()
	if notify != nil {
		notify(call)
	}
	return err
}

func (f *fakeHerder) Launches() []launchCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]launchCall(nil), f.launches...)
}

// fakeChooser answers the agent question with one scripted choice.
type fakeChooser struct {
	choice AgentChoice
	err    error
	calls  int
}

func (f *fakeChooser) Choose(context.Context) (AgentChoice, error) {
	f.calls++
	return f.choice, f.err
}

type gatedChooser struct {
	mu      sync.Mutex
	choice  AgentChoice
	calls   int
	entered chan struct{}
	allow   chan struct{}
}

func (c *gatedChooser) Choose(ctx context.Context) (AgentChoice, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if call == 1 {
		close(c.entered)
		select {
		case <-ctx.Done():
			return AgentChoice{}, ctx.Err()
		case <-c.allow:
		}
	}
	return c.choice, nil
}

func (c *gatedChooser) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type countedReader struct {
	mu sync.Mutex
	io.Reader
	reads int
}

func (r *countedReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	return r.Reader.Read(p)
}

func (r *countedReader) Reads() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reads
}

type serialTestLock struct {
	permit        chan struct{}
	secondAttempt chan struct{}
	mu            sync.Mutex
	calls         int
}

func newSerialTestLock() *serialTestLock {
	lock := &serialTestLock{permit: make(chan struct{}, 1), secondAttempt: make(chan struct{})}
	lock.permit <- struct{}{}
	return lock
}

func (l *serialTestLock) acquire(ctx context.Context, _ string) (func() error, error) {
	l.mu.Lock()
	l.calls++
	call := l.calls
	l.mu.Unlock()
	if call == 2 {
		close(l.secondAttempt)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-l.permit:
	}
	var once sync.Once
	return func() error {
		once.Do(func() { l.permit <- struct{}{} })
		return nil
	}, nil
}

// freshStartDeps drives case 0 with a scripted chooser and Herder server.
func freshStartDeps(client Herder, chooser Chooser, server Bootstrapper, diagnostics io.Writer) (StartDependencies, *[]string) {
	scoped := &[]string{}
	return StartDependencies{
		Herder:  client,
		Entropy: bytes.NewReader([]byte{1, 2, 3, 4}),
		Now:     time.Now,
		Getenv:  emptyEnv,
		Chooser: chooser,
		Scoped: func(name string) Bootstrapper {
			*scoped = append(*scoped, name)
			return server
		},
		Diagnostics: diagnostics,
	}, scoped
}

func (f *fakeHerder) Stop(_ context.Context, name string) error {
	f.mu.Lock()
	f.stops = append(f.stops, name)
	err := f.stopErrors[name]
	notify := f.onStop
	f.mu.Unlock()
	if notify != nil {
		notify(name)
	}
	return err
}

type fakeConfirmer struct {
	answer   bool
	err      error
	root     string
	names    []string
	selfStop bool
}

type lifecyclePaneResolver struct {
	pane herdr.Pane
}

func (f lifecyclePaneResolver) CurrentPane(context.Context) (herdr.Pane, error) {
	return f.pane, nil
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
	t.Cleanup(func() {
		if _, err := os.Stat(bootstrap.LogName); err == nil {
			t.Errorf("test created repository-relative %s", bootstrap.LogName)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat repository-relative %s: %v", bootstrap.LogName, err)
		}
	})
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
	sessiontest.WriteRecord(t, root, name, `{"schema_version":1,"herdr_session_name":"`+name+`","created_at":"2026-08-24T14:15:16Z"}`)
}

func emptyEnv(string) string { return "" }

func recordNamesContain(records []record.Record, name string) bool {
	for _, rec := range records {
		if rec.HerdrSessionName == name {
			return true
		}
	}
	return false
}

func TestStartAttachSkipsAgentChoiceAndBootstrap(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	writeLifecycleRecord(t, root, "registered")
	client := &fakeHerder{sessions: []herdr.Session{{Name: "registered", Running: true}}}
	chooser := &fakeChooser{}
	var diagnostics bytes.Buffer

	deps, scoped := freshStartDeps(client, chooser, sessiontest.StartedBootstrapper(), &diagnostics)
	if err := Start(context.Background(), root, deps); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if chooser.calls != 0 {
		t.Fatalf("Choose() calls = %d, want none when re-attaching", chooser.calls)
	}
	if len(*scoped) != 0 {
		t.Fatalf("bootstrapped sessions = %#v, want none when re-attaching", *scoped)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("diagnostics = %q, want empty", diagnostics.String())
	}
}

func TestStartWritesNoRecordWhenAgentChoiceFails(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	want := errors.New("selection cancelled")
	client := &fakeHerder{}
	chooser := &fakeChooser{err: want}

	deps, scoped := freshStartDeps(client, chooser, sessiontest.StartedBootstrapper(), &bytes.Buffer{})
	err := Start(context.Background(), root, deps)
	if !errors.Is(err, want) {
		t.Fatalf("Start() error = %v, want wrapped %v", err, want)
	}
	if len(client.launches) != 0 {
		t.Fatalf("launches = %#v, want none", client.launches)
	}
	if len(*scoped) != 0 {
		t.Fatalf("bootstrapped sessions = %#v, want none", *scoped)
	}
	records, err := record.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %#v, want none published for a dismissed picker", records)
	}
}

func TestStartBootstrapsFreshSessionWhileHerderRuns(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	ctx := testContext(t)
	server := sessiontest.StartedBootstrapper()
	released := make(chan struct{})
	release := closeOnCleanup(t, released)
	server.OnStart = func(context.Context, int) { release() }
	client := &fakeHerder{launchWait: released}
	chooser := &fakeChooser{choice: AgentChoice{Harness: "claude", Model: "opus"}}
	var diagnostics bytes.Buffer

	deps, scoped := freshStartDeps(client, chooser, server, &diagnostics)
	if err := Start(ctx, root, deps); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if chooser.calls != 1 {
		t.Fatalf("Choose() calls = %d, want one", chooser.calls)
	}
	name := "fledge-project-01020304"
	if want := []string{name}; !reflect.DeepEqual(*scoped, want) {
		t.Fatalf("bootstrapped sessions = %#v, want %#v", *scoped, want)
	}
	if want := []herdr.StartAgentOptions{{Name: "orchestrator", Kind: "claude", PaneID: "w1:p2", Args: []string{"--model", "opus"}}}; !reflect.DeepEqual(server.Started, want) {
		t.Fatalf("StartAgent options = %#v, want %#v", server.Started, want)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("diagnostics = %q, want empty", diagnostics.String())
	}

	log, err := os.ReadFile(filepath.Join(root, ".fledge", "sessions", name, bootstrap.LogName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "started claude") {
		t.Fatalf("bootstrap log = %q, want the agent recorded", log)
	}
}

func TestStartReportsBootstrapFailure(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	ctx := testContext(t)
	want := errors.New("agent start refused")
	server := sessiontest.StartedBootstrapper()
	server.StartErrs = []error{want}
	released := make(chan struct{})
	release := closeOnCleanup(t, released)
	// StartAgent notifies Herder before returning its substantive response. The
	// launch return therefore cancels bootstrap while this call is still ending.
	server.OnStart = func(ctx context.Context, _ int) {
		release()
		<-ctx.Done()
	}
	client := &fakeHerder{launchWait: released}
	var diagnostics bytes.Buffer

	deps, _ := freshStartDeps(client, &fakeChooser{choice: AgentChoice{Harness: "pi"}}, server, &diagnostics)
	err := Start(ctx, root, deps)
	if !errors.Is(err, want) {
		t.Fatalf("Start() error = %v, want wrapped %v", err, want)
	}
	report := diagnostics.String()
	if !strings.Contains(report, "bootstrap failed") || !strings.Contains(report, bootstrap.LogName) {
		t.Fatalf("diagnostics = %q, want a bootstrap failure naming the log", report)
	}
	if len(client.launches) != 1 {
		t.Fatalf("launches = %#v, want the session to have been launched", client.launches)
	}
}

func TestStartIgnoresBootstrapCancelledByHerderExit(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	server := &sessiontest.FakeBootstrapper{Statuses: []sessiontest.StatusResult{{Running: false}}}
	client := &fakeHerder{}
	var diagnostics bytes.Buffer

	deps, _ := freshStartDeps(client, &fakeChooser{choice: AgentChoice{Harness: "pi"}}, server, &diagnostics)
	if err := Start(context.Background(), root, deps); err != nil {
		t.Fatalf("Start() error = %v, want a cancelled bootstrap to be silent", err)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("diagnostics = %q, want empty", diagnostics.String())
	}
	if len(server.Started) != 0 {
		t.Fatalf("StartAgent calls = %#v, want none", server.Started)
	}
}

func TestStartIgnoresBootstrapSubprocessKilledByHerderExit(t *testing.T) {
	root, _ := lifecycleProject(t, "project")
	server := sessiontest.StartedBootstrapper()
	// Cancelling the bootstrap kills the herdr child, which reports the signal
	// rather than the cancellation.
	server.OnRenameWorkspace = func(ctx context.Context) error {
		<-ctx.Done()
		return contextKilledError{err: errors.New("signal: killed")}
	}
	client := &fakeHerder{}
	var diagnostics bytes.Buffer

	deps, _ := freshStartDeps(client, &fakeChooser{choice: AgentChoice{Harness: "pi"}}, server, &diagnostics)
	if err := Start(context.Background(), root, deps); err != nil {
		t.Fatalf("Start() error = %v, want a killed subprocess to be silent", err)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("diagnostics = %q, want empty", diagnostics.String())
	}
}

type contextKilledError struct {
	err error
}

func (e contextKilledError) Error() string { return e.err.Error() }

// ContextCause models the provenance recorded by the real Herdr client when
// CommandContext terminates the subprocess.
func (contextKilledError) ContextCause() error { return context.Canceled }

func TestStartClaimedWatcherCachesEarlyReleaseFailure(t *testing.T) {
	ctx := testContext(t)
	released := make(chan struct{})
	releaseLaunch := closeOnCleanup(t, released)
	releaseErr := errors.New("release failed")
	var releaseCalls int
	client := &fakeHerder{
		listResults: []listResult{{sessions: []herdr.Session{{Name: "claimed", Running: true}}}},
		launchWait:  released,
		onList: func(int) {
			releaseLaunch()
		},
	}
	launchErr := errors.New("launch failed")
	client.launchErr = launchErr
	release := func() error {
		releaseCalls++
		return releaseErr
	}

	err := startClaimed(ctx, t.TempDir(), record.Record{HerdrSessionName: "claimed", Path: t.TempDir()}, StartDependencies{Herder: client}, cachedRelease(release))
	if !errors.Is(err, launchErr) || !errors.Is(err, releaseErr) {
		t.Fatalf("startClaimed() error = %v, want launch and release failures", err)
	}
	if releaseCalls != 1 {
		t.Fatalf("release calls = %d, want one cached invocation", releaseCalls)
	}
	if got := strings.Count(err.Error(), "release project lock"); got != 1 {
		t.Fatalf("release failure occurrences = %d, want one: %v", got, err)
	}
}

func TestStartClaimedWatcherReleasesLockAfterPublicationWhileLaunchRemainsActive(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext(t))
	launchRelease := make(chan struct{})
	releaseLaunch := closeOnCleanup(t, launchRelease)
	publishExactName := make(chan struct{})
	releasePublication := closeOnCleanup(t, publishExactName)
	launchStarted := make(chan launchCall, 1)
	client := &fakeHerder{
		listResults: []listResult{
			{sessions: nil},
			{sessions: []herdr.Session{{Name: "claimed", Running: true}}},
		},
		listWait:        publishExactName,
		waitForListCall: 2,
		launchWait:      launchRelease,
		onLaunchStart: func(call launchCall) {
			launchStarted <- call
		},
	}
	lock := newSerialTestLock()
	release, err := lock.acquire(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	release = cachedRelease(release)
	errDone := make(chan error, 1)
	startWorkerErr := make(chan error, 1)
	reacquireDone := make(chan error, 1)
	reacquireWorkerErr := make(chan error, 1)
	pendingStart := 0
	pendingReacquire := 0
	root := t.TempDir()
	recordPath := t.TempDir()
	t.Cleanup(func() {
		releasePublication()
		releaseLaunch()
		cancel()
		release()
		drainTestWorkers(t, errDone, &pendingStart)
		drainTestWorkers(t, reacquireDone, &pendingReacquire)
	})
	pendingStart++
	go func() {
		err := startClaimed(ctx, root, record.Record{HerdrSessionName: "claimed", Path: recordPath}, StartDependencies{Herder: client}, release)
		startWorkerErr <- err
		errDone <- err
	}()

	awaitTestEvent(t, ctx, launchStarted, startWorkerErr)
	pendingReacquire++
	go func() {
		unlock, err := lock.acquire(ctx, "")
		if err == nil {
			unlock()
		}
		reacquireWorkerErr <- err
		reacquireDone <- err
	}()
	awaitTestEvent(t, ctx, lock.secondAttempt, reacquireWorkerErr)
	releasePublication()
	if err := awaitTestEvent(t, ctx, reacquireDone, nil); err != nil {
		t.Fatalf("project lock reacquisition error = %v", err)
	}
	pendingReacquire--
	select {
	case err := <-errDone:
		t.Fatalf("startClaimed() returned before interactive launch ended: %v", err)
	default:
	}
	releaseLaunch()
	if err := awaitTestEvent(t, ctx, errDone, nil); err != nil {
		t.Fatalf("startClaimed() error = %v", err)
	}
	pendingStart--
}

func TestStartClaimedDrainsWorkersBeforeReturn(t *testing.T) {
	ctx := testContext(t)
	watcherDone := make(chan struct{})
	bootstrapDone := make(chan struct{})
	signalWatcherDone := closeOnCleanup(t, watcherDone)
	signalBootstrapDone := closeOnCleanup(t, bootstrapDone)
	server := sessiontest.StartedBootstrapper()
	server.OnStart = func(ctx context.Context, _ int) {
		<-ctx.Done()
		signalBootstrapDone()
	}
	err := startClaimed(ctx, t.TempDir(), record.Record{
		HerdrSessionName: "claimed",
		Path:             t.TempDir(),
		PendingChoice:    &AgentChoice{Harness: "pi"},
	}, StartDependencies{
		Herder:          &drainingHerder{done: signalWatcherDone},
		Scoped:          func(string) Bootstrapper { return server },
		Diagnostics:     &bytes.Buffer{},
		bootstrapTiming: sessiontest.FastTiming(),
	}, func() error { return nil })
	if err != nil {
		t.Fatalf("startClaimed() error = %v, want cancellation-only workers to be silent", err)
	}
	select {
	case <-watcherDone:
	default:
		t.Fatal("startClaimed() returned before the watcher stopped")
	}
	select {
	case <-bootstrapDone:
	default:
		t.Fatal("startClaimed() returned before bootstrap stopped")
	}
}

func TestStartClaimedErrorOrder(t *testing.T) {
	ctx := testContext(t)
	launchErr := errors.New("launch failed")
	bootstrapErr := errors.New("bootstrap failed")
	releaseErr := errors.New("release failed")
	stateReady := make(chan struct{})
	bootReady := make(chan struct{})
	launchMayReturn := make(chan struct{})
	signalStateReady := closeOnCleanup(t, stateReady)
	signalBootReady := closeOnCleanup(t, bootReady)
	releaseLaunch := closeOnCleanup(t, launchMayReturn)
	client := &fakeHerder{
		listResults: []listResult{{sessions: []herdr.Session{{Name: "claimed", Running: true}}}},
		launchWait:  launchMayReturn,
		launchErr:   launchErr,
		onList: func(int) {
			signalStateReady()
		},
	}
	server := sessiontest.StartedBootstrapper()
	server.StartErrs = []error{bootstrapErr}
	server.OnStart = func(context.Context, int) { signalBootReady() }
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-stateReady:
		}
		select {
		case <-ctx.Done():
			return
		case <-bootReady:
		}
		releaseLaunch()
	}()
	badPath := filepath.Join(t.TempDir(), "record")
	if err := os.Mkdir(badPath, 0o700); err != nil {
		t.Fatal(err)
	}
	pendingPath := filepath.Join(badPath, "pending.json")
	if err := os.Mkdir(pendingPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pendingPath, "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	err := startClaimed(ctx, t.TempDir(), record.Record{
		HerdrSessionName: "claimed",
		Path:             badPath,
		PendingChoice:    &AgentChoice{Harness: "pi"},
	}, StartDependencies{
		Herder:          client,
		Scoped:          func(string) Bootstrapper { return server },
		Diagnostics:     &diagnostics,
		bootstrapTiming: sessiontest.FastTiming(),
	}, cachedRelease(func() error { return releaseErr }))
	if !errors.Is(err, launchErr) || !errors.Is(err, bootstrapErr) || !errors.Is(err, releaseErr) {
		t.Fatalf("startClaimed() error = %v, want launch, bootstrap, and release failures", err)
	}
	parts := []string{"launch \"claimed\"", "bootstrap: start pi", "watcher: clear pending", "release project lock"}
	previous := -1
	for _, part := range parts {
		if got := strings.Count(err.Error(), part); got != 1 {
			t.Fatalf("startClaimed() error occurrence count for %q = %d, want one: %q", part, got, err)
		}
		position := strings.Index(err.Error(), part)
		if position <= previous {
			t.Fatalf("error order = %q, want launch, bootstrap, watcher, release", err)
		}
		previous = position
	}
}

type drainingHerder struct {
	done func()
}

func (h *drainingHerder) List(ctx context.Context) ([]herdr.Session, error) {
	<-ctx.Done()
	h.done()
	return nil, ctx.Err()
}

func (*drainingHerder) Launch(context.Context, string, string) error { return nil }

func (*drainingHerder) Stop(context.Context, string) error { return nil }

func TestStartClaimedReportsCallerCancellationFromBootstrap(t *testing.T) {
	testCtx := testContext(t)
	launchMayReturn := make(chan struct{})
	bootstrapStarted := make(chan struct{})
	releaseLaunch := closeOnCleanup(t, launchMayReturn)
	signalBootstrapStarted := closeOnCleanup(t, bootstrapStarted)
	client := &fakeHerder{launchWait: launchMayReturn}
	server := &sessiontest.FakeBootstrapper{Statuses: []sessiontest.StatusResult{{Running: false}}}
	server.OnStatus = func(int) {
		signalBootstrapStarted()
	}
	ctx, cancel := context.WithCancel(testCtx)
	var diagnostics bytes.Buffer
	errDone := make(chan error, 1)
	workerErr := make(chan error, 1)
	pendingResult := 0
	root := t.TempDir()
	recordPath := t.TempDir()
	t.Cleanup(func() {
		releaseLaunch()
		cancel()
		drainTestWorkers(t, errDone, &pendingResult)
	})
	pendingResult++
	go func() {
		err := startClaimed(ctx, root, record.Record{
			HerdrSessionName: "claimed",
			Path:             recordPath,
			PendingChoice:    &AgentChoice{Harness: "pi"},
		}, StartDependencies{
			Herder:          client,
			Scoped:          func(string) Bootstrapper { return server },
			Diagnostics:     &diagnostics,
			bootstrapTiming: sessiontest.FastTiming(),
		}, func() error { return nil })
		workerErr <- err
		errDone <- err
	}()

	awaitTestEvent(t, testCtx, bootstrapStarted, workerErr)
	cancel()
	releaseLaunch()
	err := awaitTestEvent(t, testCtx, errDone, nil)
	pendingResult--
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("startClaimed() error = %v, want caller cancellation", err)
	}
	if !strings.Contains(diagnostics.String(), "bootstrap failed") {
		t.Fatalf("diagnostics = %q, want caller-cancellation report", diagnostics.String())
	}
}

func TestStartClaimedReportsBootstrapDeadline(t *testing.T) {
	ctx := testContext(t)
	deadlineReached := make(chan struct{})
	signalDeadlineReached := closeOnCleanup(t, deadlineReached)
	client := &fakeHerder{launchWait: deadlineReached}
	server := &deadlineBootstrapper{deadlineReached: signalDeadlineReached}
	var diagnostics bytes.Buffer

	err := startClaimed(ctx, t.TempDir(), record.Record{
		HerdrSessionName: "claimed",
		Path:             t.TempDir(),
		PendingChoice:    &AgentChoice{Harness: "pi"},
	}, StartDependencies{
		Herder:      client,
		Scoped:      func(string) Bootstrapper { return server },
		Diagnostics: &diagnostics,
		bootstrapTiming: bootstrap.Timing{
			Poll:         time.Millisecond,
			Deadline:     10 * time.Millisecond,
			StartRetries: 1,
			RetryDelay:   time.Millisecond,
		},
	}, func() error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("startClaimed() error = %v, want bootstrap deadline", err)
	}
	if !strings.Contains(diagnostics.String(), "bootstrap failed") {
		t.Fatalf("diagnostics = %q, want deadline report", diagnostics.String())
	}
}

type deadlineBootstrapper struct {
	deadlineReached func()
}

func (f *deadlineBootstrapper) Status(ctx context.Context) (herdr.Status, error) {
	<-ctx.Done()
	f.deadlineReached()
	return herdr.Status{}, ctx.Err()
}

func (*deadlineBootstrapper) Workspaces(context.Context) ([]herdr.Workspace, error) { return nil, nil }

func (*deadlineBootstrapper) Panes(context.Context, string) ([]herdr.Pane, error) { return nil, nil }

func (*deadlineBootstrapper) RenameWorkspace(context.Context, string, string) error { return nil }

func (*deadlineBootstrapper) RenameTab(context.Context, string, string) error { return nil }

func (*deadlineBootstrapper) StartAgent(context.Context, herdr.StartAgentOptions) (herdr.Agent, error) {
	return herdr.Agent{}, nil
}
