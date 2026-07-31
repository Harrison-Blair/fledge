package state

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestConcurrentLockedUpdatesAreNotLost(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const count = 24
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- store.WithLocked("session", "/project", func(st *Session) error {
				name := string(rune('a' + i))
				st.Agents[name] = Agent{Name: name, PaneID: name}
				return nil
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Read("session", "/project")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Agents) != count {
		t.Fatalf("persisted %d agents, want %d", len(st.Agents), count)
	}
}

func TestAtomicPersistenceAndPermissions(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	if err := store.WithLocked("s", "/p", func(st *Session) error {
		st.Agents["worker"] = Agent{Name: "worker", PaneID: "p1"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	path := store.path("s")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted Session
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("state is not valid JSON: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(root, ".*.tmp"))
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestFailedTransactionIsNotPersisted(t *testing.T) {
	store, _ := New(t.TempDir())
	sentinel := os.ErrPermission
	err := store.WithLocked("s", "/p", func(st *Session) error {
		st.Agents["bad"] = Agent{Name: "bad"}
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("error = %v", err)
	}
	st, err := store.Read("s", "/p")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Agents["bad"]; ok {
		t.Fatal("failed transaction was persisted")
	}
}

func TestLegacySchemaV1StateLoadsNewFieldsAsZeroValues(t *testing.T) {
	store, _ := New(t.TempDir())
	legacy := []byte(`{
  "schema_version": 1,
  "project_root": "/p",
  "session": "legacy",
  "agents": {}
}
`)
	if err := os.WriteFile(store.path("legacy"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Read("legacy", "/p")
	if err != nil {
		t.Fatal(err)
	}
	if st.StopGeneration != 0 {
		t.Fatalf("stop generation = %d, want 0", st.StopGeneration)
	}
	if st.OrchestratorInitialized || st.OrchestratorTabID != "" || st.OrchestratorPaneID != "" {
		t.Fatalf("legacy orchestrator state was not optional: %#v", st)
	}
}

func TestOrchestratorStatePersistsInSchemaV1(t *testing.T) {
	store, _ := New(t.TempDir())
	if err := store.WithLocked("session", "/project", func(st *Session) error {
		st.WorkspaceID = "workspace"
		st.OrchestratorTabID = "tab"
		st.OrchestratorPaneID = "pane"
		st.OrchestratorInitialized = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	st, err := store.Read("session", "/project")
	if err != nil {
		t.Fatal(err)
	}
	if st.SchemaVersion != 1 || st.OrchestratorTabID != "tab" ||
		st.OrchestratorPaneID != "pane" || !st.OrchestratorInitialized {
		t.Fatalf("persisted orchestrator state = %#v", st)
	}
}

func TestReadExistingMissingSessionDoesNotCreateFiles(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	st, found, err := store.ReadExisting("missing", "/project")
	if err != nil || found {
		t.Fatalf("missing read = %#v, found=%t, err=%v", st, found, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("read-only lookup created files: %v", entries)
	}
}

func TestReadExistingDoesNotRewriteOrNormalizeState(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := store.path("session")
	before := []byte("{\n  \"schema_version\": 1,\n  \"project_root\": \"/project\",\n  \"session\": \"session\"\n}")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}

	st, found, err := store.ReadExisting("session", "/project")
	if err != nil || !found {
		t.Fatalf("existing read found=%t err=%v", found, err)
	}
	if st.Agents != nil {
		t.Fatalf("read-only lookup normalized omitted agents: %#v", st.Agents)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("read-only lookup rewrote state:\nbefore=%q\nafter=%q", before, after)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("read-only lookup created a lock file: %v", err)
	}
}

func TestReadExistingValidatesStateWithoutRewritingIt(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "malformed",
			content: "{broken\n",
			want:    "decode state",
		},
		{
			name:    "schema",
			content: `{"schema_version":2,"project_root":"/project","session":"session","agents":{}}`,
			want:    "unsupported state schema 2",
		},
		{
			name:    "project",
			content: `{"schema_version":1,"project_root":"/other","session":"session","agents":{}}`,
			want:    `session "session" belongs to project /other`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := store.path("session")
			before := []byte(test.content)
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, found, err := store.ReadExisting("session", "/project"); !found ||
				err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("found=%t err=%v, want error containing %q", found, err, test.want)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("failed read rewrote state:\nbefore=%q\nafter=%q", before, after)
			}
		})
	}
}
