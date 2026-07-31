package fledge

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/herdrtest"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

func TestStopDeletesStoppedSessionAndClearsMappings(t *testing.T) {
	service, exists, _ := newStoppedService(t, true, false)
	seedDisposableState(t, service, 7)

	result, err := service.Stop(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stopped || !result.Deleted || result.Forced {
		t.Fatalf("unexpected stop result: %#v", result)
	}
	if _, err := os.Stat(exists); !os.IsNotExist(err) {
		t.Fatalf("session namespace remains: %v", err)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	assertDisposableStateCleared(t, st, 7)
}

func TestStopMissingSessionIsSuccessfulAndClearsStaleMappings(t *testing.T) {
	service, _, log := newStoppedService(t, false, false)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.StopGeneration = 3
		st.Socket = "/stale/socket"
		st.Agents["worker"] = state.Agent{Name: "worker"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	result, err := service.Stop(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stopped || result.Deleted || !result.Forced {
		t.Fatalf("unexpected missing-session result: %#v", result)
	}
	if strings.Contains(readStopLog(t, log), "session delete") {
		t.Fatalf("missing session was deleted: %s", readStopLog(t, log))
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.StopGeneration != 3 || st.Socket != "" || len(st.Agents) != 0 {
		t.Fatalf("stale state was not cleared: %#v", st)
	}
}

func TestStopDeleteFailureCanBeRetried(t *testing.T) {
	service, exists, _ := newStoppedService(t, true, true)
	_, err := service.Stop(t.Context(), false)
	translated := Translate(err)
	if translated.Code != "session_delete_failed" {
		t.Fatalf("first stop error = %#v", translated)
	}
	details, ok := translated.Details.(StopResult)
	if !ok || details.Stopped || details.Deleted {
		t.Fatalf("delete failure details = %#v", translated.Details)
	}
	if _, statErr := os.Stat(exists); statErr != nil {
		t.Fatalf("failed deletion removed session: %v", statErr)
	}

	result, err := service.Stop(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deleted || result.Stopped {
		t.Fatalf("retry result = %#v", result)
	}
}

func TestConcurrentStopFinalizersDeleteIdempotentlyAndAdvanceGenerationOnce(t *testing.T) {
	service, exists, _ := newStoppedService(t, true, false)
	seedDisposableState(t, service, 7)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	finalizers := []*Service{service, {
		Project: service.Project,
		Binary:  service.Binary,
		Store:   service.Store,
	}}
	for _, finalizer := range finalizers {
		go func(finalizer *Service) {
			ready.Done()
			<-start
			errs <- finalizer.FinalizeStop(context.Background(), 7, time.Second)
		}(finalizer)
	}
	ready.Wait()
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(exists); !os.IsNotExist(err) {
		t.Fatalf("session namespace remains: %v", err)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	assertDisposableStateCleared(t, st, 8)
}

func TestPrepareFreshStartDeletesStoppedSessionAndClearsMappings(t *testing.T) {
	service, exists, _ := newStoppedService(t, true, false)
	seedDisposableState(t, service, 11)

	if err := service.prepareFreshStart(t.Context(), herdr.SessionInfo{
		Name: service.Project.Session, Running: false,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(exists); !os.IsNotExist(err) {
		t.Fatalf("stopped session namespace remains: %v", err)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	assertDisposableStateCleared(t, st, 11)
}

func TestPrepareFreshStartAbortsWhenStoppedSessionCannotBeDeleted(t *testing.T) {
	service, exists, _ := newStoppedService(t, true, true)
	err := service.prepareFreshStart(t.Context(), herdr.SessionInfo{
		Name: service.Project.Session, Running: false,
	})
	if translated := Translate(err); translated.Code != "session_delete_failed" ||
		!strings.Contains(translated.Message, "before startup") {
		t.Fatalf("unexpected cleanup error: %#v", translated)
	}
	if _, err := os.Stat(exists); err != nil {
		t.Fatalf("failed startup cleanup removed session: %v", err)
	}
}

func TestPrepareFreshStartAbortsWhenStaleMappingsCannotBeCleared(t *testing.T) {
	service, _, _ := newStoppedService(t, false, false)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.StopGeneration = 4
		st.Socket = "/stale/socket"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	corruptStateFile(t, service)

	err := service.prepareFreshStart(t.Context(), herdr.SessionInfo{})
	if translated := Translate(err); translated.Code != "state_persist_failed" ||
		!strings.Contains(translated.Message, "before startup") {
		t.Fatalf("unexpected state cleanup error: %#v", translated)
	}
}

func TestStoppedSessionCleanupReportsUnreadableStateAsPersistFailure(t *testing.T) {
	service, exists, _ := newStoppedService(t, true, false)
	corruptStateFile(t, service)

	_, err := service.Stop(t.Context(), false)
	translated := Translate(err)
	if translated.Code != "state_persist_failed" ||
		!strings.Contains(translated.Message, "active message run could not be closed") {
		t.Fatalf("stopped-session cleanup error = %#v", translated)
	}
	if _, statErr := os.Stat(exists); !os.IsNotExist(statErr) {
		t.Fatalf("session was not deleted before the state failure: %v", statErr)
	}
}

func TestStoppedSessionCleanupSurfacesCorruptMessageLog(t *testing.T) {
	service, _, _ := newStoppedService(t, true, false)
	corruptMessageRun(t, service, startTestMessageRun(t, service))

	_, err := service.Stop(t.Context(), false)
	if translated := Translate(err); translated.Code != "message_log_corrupt" {
		t.Fatalf("stopped-session cleanup error = %#v", translated)
	}
}

func TestPrepareFreshStartSurfacesCorruptMessageLog(t *testing.T) {
	service, _, _ := newStoppedService(t, false, false)
	corruptMessageRun(t, service, startTestMessageRun(t, service))

	err := service.prepareFreshStart(t.Context(), herdr.SessionInfo{})
	if translated := Translate(err); translated.Code != "message_log_corrupt" {
		t.Fatalf("fresh-start error = %#v", translated)
	}
}

// corruptStateFile replaces the persisted session with unparseable JSON. The
// leading Read is load-bearing: it materializes the state file, and the glob
// below asserts exactly one exists, so a caller cannot silently "corrupt"
// nothing and still see the error it expected from some other cause.
func corruptStateFile(t *testing.T, service *Service) {
	t.Helper()
	if _, err := service.Store.Read(service.Project.Session, service.Project.Root); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(service.Store.Root, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("state files = %v, %v", files, err)
	}
	if err := os.WriteFile(files[0], []byte("{broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newStoppedService(t *testing.T, exists, failOnce bool) (*Service, string, string) {
	t.Helper()
	temp := t.TempDir()
	existsPath := filepath.Join(temp, "exists")
	failPath := filepath.Join(temp, "fail-once")
	log := filepath.Join(temp, "invocations")
	if exists {
		if err := os.WriteFile(existsPath, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if failOnce {
		if err := os.WriteFile(failPath, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	session := "fledge-test-deadbeef"
	payload, err := json.Marshal(struct {
		Sessions []herdr.SessionInfo `json:"sessions"`
	}{Sessions: []herdr.SessionInfo{{Name: session, Running: false}}})
	if err != nil {
		t.Fatal(err)
	}
	binaryPath := herdrtest.WriteBinary(t, temp, herdrtest.Options{
		InvocationLog:  log,
		Sessions:       []herdrtest.SessionCase{{Marker: existsPath, Payload: string(payload)}},
		DeleteRemoves:  existsPath,
		DeleteFailOnce: failPath,
	})
	store, err := state.New(filepath.Join(temp, "state"))
	if err != nil {
		t.Fatal(err)
	}
	return &Service{
		Project: project.Info{Root: t.TempDir(), Session: session, SessionSource: "derived"},
		Binary:  herdr.Binary{Path: binaryPath},
		Store:   store,
		Installed: &herdr.BinaryInfo{
			Path: binaryPath, Version: "herdr test", Protocol: 17,
		},
	}, existsPath, log
}

func readStopLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes.TrimSpace(data)) + "\n"
}
