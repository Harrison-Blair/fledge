package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type counter struct {
	N int `json:"n"`
}

func openStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "state")
	store, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return store, root
}

func createCounter(t *testing.T, store *Store) string {
	t.Helper()
	id, err := store.Create("counters", func(string) any { return counter{} })
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return id
}

func increment(store *Store, id string) error {
	var c counter
	return store.Update("counters", id, &c, func() error {
		c.N++
		return nil
	})
}

func TestOpenCreatesPrivateRootAndLock(t *testing.T) {
	_, root := openStore(t)
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("root mode = %v, want directory 0700", info.Mode())
	}
	info, err = os.Stat(filepath.Join(root, lockName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("lock mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestCreateGetRoundTrip(t *testing.T) {
	store, root := openStore(t)
	type agent struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	id, err := store.Create("agents", func(id string) any { return agent{ID: id, Name: "reviewer"} })
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !ValidID(id) {
		t.Fatalf("id %q is not 8 lowercase hex characters", id)
	}
	var got agent
	if err := store.Get("agents", id, &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != (agent{ID: id, Name: "reviewer"}) {
		t.Fatalf("Get = %+v", got)
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(root, "agents"):             0o700,
		filepath.Join(root, "agents", id+".json"): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", path, info.Mode().Perm(), want)
		}
	}
}

func TestGetMissingReturnsNotFound(t *testing.T) {
	store, _ := openStore(t)
	var c counter
	err := store.Get("counters", "0123abcd", &c)
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("Get error = %v, want *NotFoundError", err)
	}
	if notFound.Kind != "counters" || notFound.ID != "0123abcd" {
		t.Fatalf("NotFoundError = %+v", notFound)
	}
}

func TestUpdateMissingReturnsNotFound(t *testing.T) {
	store, _ := openStore(t)
	var c counter
	err := store.Update("counters", "0123abcd", &c, func() error { return nil })
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("Update error = %v, want *NotFoundError", err)
	}
}

func TestCreateCollisionRetriesWithoutOverwriting(t *testing.T) {
	store, _ := openStore(t)
	ids := []string{"aaaaaaaa", "aaaaaaaa", "bbbbbbbb"}
	store.newID = func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	first, err := store.Create("counters", func(string) any { return counter{N: 1} })
	if err != nil || first != "aaaaaaaa" {
		t.Fatalf("first Create = %q, %v", first, err)
	}
	second, err := store.Create("counters", func(string) any { return counter{N: 2} })
	if err != nil || second != "bbbbbbbb" {
		t.Fatalf("second Create = %q, %v; want bbbbbbbb", second, err)
	}
	var c counter
	if err := store.Get("counters", first, &c); err != nil || c.N != 1 {
		t.Fatalf("first record = %+v, %v; want N=1", c, err)
	}
	if err := store.Get("counters", second, &c); err != nil || c.N != 2 {
		t.Fatalf("second record = %+v, %v; want N=2", c, err)
	}
}

func TestCreateGivesUpAfterBoundedCollisions(t *testing.T) {
	store, _ := openStore(t)
	store.newID = func() (string, error) { return "aaaaaaaa", nil }
	if _, err := store.Create("counters", func(string) any { return counter{N: 1} }); err != nil {
		t.Fatal(err)
	}
	calls := 0
	store.newID = func() (string, error) {
		calls++
		return "aaaaaaaa", nil
	}
	if _, err := store.Create("counters", func(string) any { return counter{N: 2} }); err == nil {
		t.Fatal("Create succeeded despite every id colliding")
	}
	if calls != createAttempts {
		t.Fatalf("id source called %d times, want %d", calls, createAttempts)
	}
	var c counter
	if err := store.Get("counters", "aaaaaaaa", &c); err != nil || c.N != 1 {
		t.Fatalf("record = %+v, %v; want untouched N=1", c, err)
	}
}

func TestCreateNeverReusesAnArchivedID(t *testing.T) {
	store, _ := openStore(t)
	ids := []string{"aaaaaaaa", "aaaaaaaa", "bbbbbbbb"}
	store.newID = func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	first, err := store.Create("counters", func(string) any { return counter{N: 1} })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Exclusive(func(tx *Tx) error { return tx.Archive("counters", first) }); err != nil {
		t.Fatal(err)
	}
	// The live path is free again, but the id still names the archived record.
	second, err := store.Create("counters", func(string) any { return counter{N: 2} })
	if err != nil || second != "bbbbbbbb" {
		t.Fatalf("second Create = %q, %v; want bbbbbbbb", second, err)
	}
	var c counter
	if err := store.Get("counters", first, &c); err != nil || c.N != 1 {
		t.Fatalf("archived record = %+v, %v; want N=1", c, err)
	}
}

func TestUnlockedCreateRacingArchiveNeverReusesTheID(t *testing.T) {
	store, _ := openStore(t)
	ids := []string{"aaaaaaaa", "aaaaaaaa", "bbbbbbbb"}
	store.newID = func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	first, err := store.Create("counters", func(string) any { return counter{N: 1} })
	if err != nil {
		t.Fatal(err)
	}
	// The colliding record is archived between Create's two steps, which
	// checking the archive before claiming the live path would miss.
	archived := false
	createStep = func() {
		if !archived {
			archived = true
			if err := store.Exclusive(func(tx *Tx) error { return tx.Archive("counters", first) }); err != nil {
				t.Error(err)
			}
		}
	}
	t.Cleanup(func() { createStep = func() {} })
	second, err := store.Create("counters", func(string) any { return counter{N: 2} })
	if err != nil || second != "bbbbbbbb" || !archived {
		t.Fatalf("second Create = %q, %v (archived %v); want bbbbbbbb", second, err, archived)
	}
	var c counter
	if err := store.Get("counters", first, &c); err != nil || c.N != 1 {
		t.Fatalf("archived record = %+v, %v; want N=1", c, err)
	}
}

func TestArchiveRefusesToOverwriteAnArchivedRecord(t *testing.T) {
	store, root := openStore(t)
	id := createCounter(t, store)
	if err := store.Exclusive(func(tx *Tx) error { return tx.Archive("counters", id) }); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(root, "counters", id+recordSuffix)
	if err := os.WriteFile(live, []byte(`{"n": 9}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Exclusive(func(tx *Tx) error { return tx.Archive("counters", id) }); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("Archive over an archived record: %v, want fs.ErrExist", err)
	}
	var c counter
	data, _ := os.ReadFile(filepath.Join(root, "counters", archiveDir, id+recordSuffix))
	if err := json.Unmarshal(data, &c); err != nil || c.N != 0 {
		t.Fatalf("archived record = %s, %v; want untouched", data, err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("refused Archive removed the live record: %v", err)
	}
}

func TestArchiveCompletesAnInterruptedMove(t *testing.T) {
	store, root := openStore(t)
	id := createCounter(t, store)
	live := filepath.Join(root, "counters", id+recordSuffix)
	// A move interrupted after linking leaves the record at both paths.
	if err := os.Mkdir(filepath.Join(root, "counters", archiveDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(live, filepath.Join(root, "counters", archiveDir, id+recordSuffix)); err != nil {
		t.Fatal(err)
	}
	if err := store.Exclusive(func(tx *Tx) error { return tx.Archive("counters", id) }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(live); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("live path remains: %v", err)
	}
}

func TestUnarchiveReturnsRecordToList(t *testing.T) {
	store, root := openStore(t)
	id := createCounter(t, store)
	unarchive := func(id string) error {
		return store.Exclusive(func(tx *Tx) error { return tx.Unarchive("counters", id) })
	}
	if err := unarchive(id); err != nil { // a live record is left as is
		t.Fatal(err)
	}
	if err := store.Exclusive(func(tx *Tx) error { return tx.Archive("counters", id) }); err != nil {
		t.Fatal(err)
	}
	if err := unarchive(id); err != nil {
		t.Fatal(err)
	}
	if ids, err := store.List("counters"); err != nil || !reflect.DeepEqual(ids, []string{id}) {
		t.Fatalf("List = %v, %v", ids, err)
	}
	if _, err := os.Stat(filepath.Join(root, "counters", archiveDir, id+recordSuffix)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("archived copy remains: %v", err)
	}
	var notFound *NotFoundError
	if err := unarchive("0123abcd"); !errors.As(err, &notFound) {
		t.Fatalf("Unarchive of a missing record: %v, want *NotFoundError", err)
	}
}

func TestListReturnsSortedIDsAndIgnoresOtherFiles(t *testing.T) {
	store, root := openStore(t)
	if ids, err := store.List("counters"); err != nil || len(ids) != 0 {
		t.Fatalf("List of missing kind = %v, %v; want empty", ids, err)
	}
	ids := []string{"cccccccc", "aaaaaaaa", "bbbbbbbb"}
	store.newID = func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	for range 3 {
		createCounter(t, store)
	}
	dir := filepath.Join(root, "counters")
	for _, name := range []string{".tmp-123", "notes.txt", "ABCDEF01.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.List("counters")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"aaaaaaaa", "bbbbbbbb", "cccccccc"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
}

func TestUpdatePersistsMutation(t *testing.T) {
	store, _ := openStore(t)
	id := createCounter(t, store)
	if err := increment(store, id); err != nil {
		t.Fatal(err)
	}
	var c counter
	if err := store.Get("counters", id, &c); err != nil || c.N != 1 {
		t.Fatalf("after Update = %+v, %v; want N=1", c, err)
	}
}

func TestUpdateMutateErrorLeavesRecordUnchanged(t *testing.T) {
	store, _ := openStore(t)
	id := createCounter(t, store)
	boom := errors.New("boom")
	var c counter
	err := store.Update("counters", id, &c, func() error {
		c.N = 99
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Update error = %v, want boom", err)
	}
	var got counter
	if err := store.Get("counters", id, &got); err != nil || got.N != 0 {
		t.Fatalf("record = %+v, %v; want unchanged N=0", got, err)
	}
}

func TestMalformedRecordErrorNamesFile(t *testing.T) {
	store, root := openStore(t)
	id := createCounter(t, store)
	path := filepath.Join(root, "counters", id+".json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	var c counter
	if err := store.Get("counters", id, &c); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Get error = %v, want error naming %s", err, path)
	}
	called := false
	err := store.Update("counters", id, &c, func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Update error = %v, want error naming %s", err, path)
	}
	if called {
		t.Fatal("Update ran mutate on a malformed record")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{not json" {
		t.Fatalf("malformed record was rewritten: %q, %v", data, err)
	}
}

func TestLeftoverTempFileIsTolerated(t *testing.T) {
	_, root := openStore(t)
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	id := createCounter(t, store)
	// Simulate a writer that crashed after creating its temp file but before
	// renaming it over the record.
	dir := filepath.Join(root, "counters")
	if err := os.WriteFile(filepath.Join(dir, tempPrefix+"crashed"), []byte(`{"n":`), 0o600); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(root)
	if err != nil {
		t.Fatalf("Open after crash: %v", err)
	}
	var c counter
	if err := reopened.Get("counters", id, &c); err != nil || c.N != 0 {
		t.Fatalf("Get after crash = %+v, %v", c, err)
	}
	if err := increment(reopened, id); err != nil {
		t.Fatalf("Update after crash: %v", err)
	}
	ids, err := reopened.List("counters")
	if err != nil || !reflect.DeepEqual(ids, []string{id}) {
		t.Fatalf("List after crash = %v, %v", ids, err)
	}
}

func TestInvalidKindAndIDRejected(t *testing.T) {
	store, _ := openStore(t)
	var c counter
	for _, kind := range []string{"", ".", "..", "a/b", "../x"} {
		if _, err := store.Create(kind, func(string) any { return c }); err == nil {
			t.Errorf("Create(%q) succeeded", kind)
		}
		if _, err := store.List(kind); err == nil {
			t.Errorf("List(%q) succeeded", kind)
		}
	}
	for _, id := range []string{"", "0123abc", "0123ABCD", "../../x", "0123abcde"} {
		err := store.Get("counters", id, &c)
		var notFound *NotFoundError
		if err == nil || errors.As(err, &notFound) {
			t.Errorf("Get(%q) error = %v, want invalid id error", id, err)
		}
	}
}

func TestConcurrentUpdatesDoNotLoseWrites(t *testing.T) {
	store, root := openStore(t)
	id := createCounter(t, store)
	other, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	const n = 200
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, s := range []*Store{store, other} {
		wg.Go(func() {
			for range n {
				if err := increment(s, id); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var c counter
	if err := store.Get("counters", id, &c); err != nil || c.N != 2*n {
		t.Fatalf("counter = %+v, %v; want %d", c, err, 2*n)
	}
}

const (
	helperRootEnv  = "FLEDGE_STATE_HELPER_ROOT"
	helperIDEnv    = "FLEDGE_STATE_HELPER_ID"
	helperCountEnv = "FLEDGE_STATE_HELPER_COUNT"
)

// TestHelperProcessIncrement is re-executed by
// TestConcurrentProcessesDoNotLoseWrites and is a no-op otherwise.
func TestHelperProcessIncrement(t *testing.T) {
	root := os.Getenv(helperRootEnv)
	if root == "" {
		t.Skip("helper process only")
	}
	count, err := strconv.Atoi(os.Getenv(helperCountEnv))
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for range count {
		if err := increment(store, os.Getenv(helperIDEnv)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConcurrentProcessesDoNotLoseWrites(t *testing.T) {
	if os.Getenv(helperRootEnv) != "" {
		t.Skip("inside helper process")
	}
	store, root := openStore(t)
	id := createCounter(t, store)
	const n = 200
	cmds := make([]*exec.Cmd, 2)
	for i := range cmds {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcessIncrement$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			helperRootEnv+"="+root,
			helperIDEnv+"="+id,
			fmt.Sprintf("%s=%d", helperCountEnv, n),
		)
		cmds[i] = cmd
	}
	outputs := make([][]byte, len(cmds))
	errs := make([]error, len(cmds))
	var wg sync.WaitGroup
	for i, cmd := range cmds {
		wg.Go(func() { outputs[i], errs[i] = cmd.CombinedOutput() })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("helper %d: %v\n%s", i, err, outputs[i])
		}
	}
	var c counter
	if err := store.Get("counters", id, &c); err != nil || c.N != 2*n {
		t.Fatalf("counter = %+v, %v; want %d", c, err, 2*n)
	}
}

// recordDirOps routes directory creation and syncing through the real
// operations while logging each call as "mkdir <path>" or "sync <path>".
func recordDirOps(t *testing.T, mkdirErr func(path string) error) *[]string {
	t.Helper()
	var ops []string
	realMkdir, realSync := mkdir, syncDir
	mkdir = func(path string, perm os.FileMode) error {
		ops = append(ops, "mkdir "+path)
		if err := realMkdir(path, perm); err != nil {
			return err
		}
		if mkdirErr != nil {
			return mkdirErr(path)
		}
		return nil
	}
	syncDir = func(path string) error {
		ops = append(ops, "sync "+path)
		return realSync(path)
	}
	t.Cleanup(func() { mkdir, syncDir = realMkdir, realSync })
	return &ops
}

func TestMovesSyncTheNewPathBeforeRemovingTheOld(t *testing.T) {
	store, root := openStore(t)
	id := createCounter(t, store)
	kind, archive := filepath.Join(root, "counters"), filepath.Join(root, "counters", archiveDir)
	live, archived := filepath.Join(kind, id+recordSuffix), filepath.Join(archive, id+recordSuffix)
	ops := recordDirOps(t, nil)
	realSync := syncDir
	// Log which record paths exist as each sync starts: an unlink synced
	// before the link would lose the record in a crash.
	syncDir = func(path string) error {
		for _, p := range []string{live, archived} {
			if _, err := os.Stat(p); err == nil {
				*ops = append(*ops, "  has "+p)
			}
		}
		return realSync(path)
	}
	if err := store.Exclusive(func(tx *Tx) error { return tx.Archive("counters", id) }); err != nil {
		t.Fatal(err)
	}
	// The new archive directory is synced into the kind directory before it
	// holds the only copy of the record.
	want := []string{"mkdir " + archive, "  has " + live, "sync " + kind, "  has " + live, "  has " + archived, "sync " + archive, "  has " + archived, "sync " + kind}
	if !reflect.DeepEqual(*ops, want) {
		t.Fatalf("Archive ops = %q\nwant %q", *ops, want)
	}
	*ops = nil
	if err := store.Exclusive(func(tx *Tx) error { return tx.Unarchive("counters", id) }); err != nil {
		t.Fatal(err)
	}
	want = []string{"  has " + live, "  has " + archived, "sync " + kind, "  has " + live, "sync " + archive}
	if !reflect.DeepEqual(*ops, want) {
		t.Fatalf("Unarchive ops = %q\nwant %q", *ops, want)
	}
}

func TestOpenAndCreateSyncParentsOfNewDirectories(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "state")
	ops := recordDirOps(t, nil)

	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"mkdir " + root, "sync " + parent, "sync " + root}; !reflect.DeepEqual(*ops, want) {
		t.Fatalf("Open ops = %q\nwant %q", *ops, want)
	}

	*ops = nil
	createCounter(t, store)
	kind := filepath.Join(root, "counters")
	if want := []string{"mkdir " + kind, "sync " + root, "sync " + kind}; !reflect.DeepEqual(*ops, want) {
		t.Fatalf("Create ops = %q\nwant %q", *ops, want)
	}
}

func TestOpenRequiresExistingParent(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing")
	root := filepath.Join(parent, "state")
	_, err := Open(root)
	if err == nil || !strings.Contains(err.Error(), parent) {
		t.Fatalf("Open error = %v, want error naming %s", err, parent)
	}
	if _, err := os.Stat(parent); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open created the missing parent: stat error = %v", err)
	}
}

// TestSyncsDoNotDependOnWhoCreatedDirectories covers a process that finds the
// directories already present, possibly created by another process that has
// not synced them yet.
func TestSyncsDoNotDependOnWhoCreatedDirectories(t *testing.T) {
	store, root := openStore(t)
	createCounter(t, store)
	ops := recordDirOps(t, nil)

	if _, err := Open(root); err != nil {
		t.Fatal(err)
	}
	if want := []string{"sync " + filepath.Dir(root), "sync " + root}; !reflect.DeepEqual(*ops, want) {
		t.Fatalf("Open ops = %q, want %q", *ops, want)
	}

	*ops = nil
	createCounter(t, store)
	kind := filepath.Join(root, "counters")
	if want := []string{"sync " + root, "sync " + kind}; !reflect.DeepEqual(*ops, want) {
		t.Fatalf("Create ops = %q, want %q", *ops, want)
	}
}

func TestConcurrentDirectoryCreatorStillSyncsParent(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "state")
	// The directory appears between the existence check and Mkdir, as when
	// another process wins the race.
	ops := recordDirOps(t, func(path string) error {
		return &os.PathError{Op: "mkdir", Path: path, Err: fs.ErrExist}
	})
	if _, err := Open(root); err != nil {
		t.Fatalf("Open with racing creator: %v", err)
	}
	want := []string{"mkdir " + root, "sync " + base, "sync " + root}
	if !reflect.DeepEqual(*ops, want) {
		t.Fatalf("ops = %q, want %q", *ops, want)
	}
}

func TestOpenRejectsNonDirectoryRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(root, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(root)
	if !errors.Is(err, syscall.ENOTDIR) || !strings.Contains(err.Error(), "create "+root+":") {
		t.Fatalf("Open error = %v, want ENOTDIR creating %s", err, root)
	}
}

func TestOpenRejectsDirectoryLock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(filepath.Join(root, lockName), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root); err == nil {
		t.Fatal("Open succeeded with a directory in place of the lock file")
	}
}

func TestExclusiveBlocksOtherHolders(t *testing.T) {
	store, root := openStore(t)
	other, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- store.Exclusive(func(*Tx) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	ran := make(chan error, 1)
	go func() { ran <- other.Exclusive(func(*Tx) error { return nil }) }()
	select {
	case err := <-ran:
		t.Fatalf("second Exclusive ran while the first held the lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-ran; err != nil {
		t.Fatal(err)
	}
}

func TestExclusiveReturnsFnErrorAndReleases(t *testing.T) {
	store, _ := openStore(t)
	sentinel := errors.New("refused")
	if err := store.Exclusive(func(*Tx) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	if err := store.Exclusive(func(*Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestTxPutReplacesRecordsUnderTheLock(t *testing.T) {
	store, _ := openStore(t)
	id := createCounter(t, store)
	err := store.Exclusive(func(tx *Tx) error {
		var c counter
		if err := tx.Get("counters", id, &c); err != nil {
			return err
		}
		c.N = 7
		return tx.Put("counters", id, c)
	})
	var c counter
	if err != nil || store.Get("counters", id, &c) != nil || c.N != 7 {
		t.Fatalf("n = %d, %v", c.N, err)
	}
	var notFound *NotFoundError
	err = store.Exclusive(func(tx *Tx) error { return tx.Put("counters", "0123abcd", counter{}) })
	if !errors.As(err, &notFound) {
		t.Fatalf("Put of a missing record: %v, want *NotFoundError", err)
	}
	if _, err := os.Stat(filepath.Join(store.root, "counters", "0123abcd.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Put created a missing record: %v", err)
	}
}

func TestArchiveHidesRecordFromListButKeepsItReadable(t *testing.T) {
	store, root := openStore(t)
	kept, archived := createCounter(t, store), createCounter(t, store)
	for range 2 { // archiving an archived record is a no-op
		if err := store.Exclusive(func(tx *Tx) error { return tx.Archive("counters", archived) }); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "counters", archiveDir, archived+recordSuffix)); err != nil {
		t.Fatal(err)
	}
	if ids, err := store.List("counters"); err != nil || !reflect.DeepEqual(ids, []string{kept}) {
		t.Fatalf("List = %v, %v; want only %s", ids, err, kept)
	}
	// Archived records stay readable and updatable by id, in the archive.
	if err := increment(store, archived); err != nil {
		t.Fatal(err)
	}
	var c counter
	if err := store.Get("counters", archived, &c); err != nil || c.N != 1 {
		t.Fatalf("n = %d, %v", c.N, err)
	}
	if _, err := os.Stat(filepath.Join(root, "counters", archived+recordSuffix)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Update republished the archived record: %v", err)
	}
	var notFound *NotFoundError
	if err := store.Exclusive(func(tx *Tx) error { return tx.Archive("counters", "0123abcd") }); !errors.As(err, &notFound) {
		t.Fatalf("Archive of a missing record: %v, want *NotFoundError", err)
	}
}

// claimNext creates a "claims" record numbered by how many already exist,
// inside one exclusive section, so overlapping sections would reuse a number.
func claimNext(s *Store) error {
	return s.Exclusive(func(tx *Tx) error {
		ids, err := tx.List("claims")
		if err != nil {
			return err
		}
		_, err = tx.Create("claims", func(string) any { return counter{N: len(ids)} })
		return err
	})
}

func checkClaims(t *testing.T, s *Store, want int) {
	t.Helper()
	ids, err := s.List("claims")
	if err != nil || len(ids) != want {
		t.Fatalf("%d claims, %v; want %d", len(ids), err, want)
	}
	seen := map[int]bool{}
	for _, id := range ids {
		var c counter
		if err := s.Get("claims", id, &c); err != nil {
			t.Fatal(err)
		}
		if seen[c.N] {
			t.Fatalf("claim number %d reused: sections overlapped", c.N)
		}
		seen[c.N] = true
	}
}

const helperClaimsEnv = "FLEDGE_STATE_HELPER_CLAIMS"

// TestHelperProcessClaim is re-executed by
// TestExclusiveSerializesProcesses and is a no-op otherwise.
func TestHelperProcessClaim(t *testing.T) {
	root := os.Getenv(helperClaimsEnv)
	if root == "" {
		t.Skip("helper process only")
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for range 100 {
		if err := claimNext(store); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExclusiveSerializesProcesses(t *testing.T) {
	if os.Getenv(helperClaimsEnv) != "" {
		t.Skip("inside helper process")
	}
	store, root := openStore(t)
	outputs := make([][]byte, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range outputs {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcessClaim$", "-test.count=1")
		cmd.Env = append(os.Environ(), helperClaimsEnv+"="+root)
		wg.Go(func() { outputs[i], errs[i] = cmd.CombinedOutput() })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("helper %d: %v\n%s", i, err, outputs[i])
		}
	}
	checkClaims(t, store, 200)
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func TestOpenExistingCreatesNothing(t *testing.T) {
	store, root := openStore(t)
	id := createCounter(t, store)
	if err := os.Remove(filepath.Join(root, lockName)); err != nil {
		t.Fatal(err)
	}
	before := entries(t, root)
	existing, err := OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	var c counter
	if ids, err := existing.List("counters"); err != nil || !reflect.DeepEqual(ids, []string{id}) || existing.Get("counters", id, &c) != nil {
		t.Fatalf("List = %v %v", ids, err)
	}
	if after := entries(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("entries %v, want %v", after, before)
	}
	// Writes still work, taking the lock lazily.
	if err := increment(existing, id); err != nil {
		t.Fatal(err)
	}
}

func TestOpenExistingRejectsMissingOrNonDirectoryRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "state")
	if _, err := OpenExisting(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("OpenExisting missing = %v, want not exist", err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("OpenExisting created %s", missing)
	}
	file := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenExisting(file); !errors.Is(err, syscall.ENOTDIR) {
		t.Fatalf("OpenExisting file = %v, want ENOTDIR", err)
	}
}

func TestValidID(t *testing.T) {
	for id, want := range map[string]bool{
		"0123abcd": true, "56789def": true, "": false, "0123abc": false, "0123abcde": false,
		"0123ABCD": false, "0123abcg": false, "0123abc\n": false, "0123abcé": false,
	} {
		if got := ValidID(id); got != want {
			t.Errorf("ValidID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestListArchivedReturnsOnlyArchivedIDs(t *testing.T) {
	store, _ := openStore(t)
	if ids, err := store.ListArchived("counters"); err != nil || len(ids) != 0 || ids == nil {
		t.Fatalf("ListArchived before any records = %#v, %v", ids, err)
	}
	kept, archived := createCounter(t, store), createCounter(t, store)
	if ids, err := store.ListArchived("counters"); err != nil || len(ids) != 0 {
		t.Fatalf("ListArchived before archiving = %v, %v", ids, err)
	}
	if err := store.Exclusive(func(tx *Tx) error { return tx.Archive("counters", archived) }); err != nil {
		t.Fatal(err)
	}
	if ids, err := store.ListArchived("counters"); err != nil || !reflect.DeepEqual(ids, []string{archived}) {
		t.Fatalf("ListArchived = %v, %v; want only %s, not %s", ids, err, archived, kept)
	}
}
