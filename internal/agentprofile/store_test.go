package agentprofile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestNoDirectoryBehavior(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t, root)

	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if profiles == nil || len(profiles) != 0 {
		t.Fatalf("List() = %#v, want non-nil empty slice", profiles)
	}
	if _, err := os.Lstat(filepath.Join(root, ".fledge")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read-only List created .fledge: %v", err)
	}

	for op, err := range map[string]error{
		"load":   loadError(store, "missing"),
		"update": updateError(store, testProfile("missing")),
		"delete": store.Delete("missing"),
	} {
		assertKind(t, err, ErrNotFound, KindNotFound)
		t.Logf("%s: %v", op, err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".fledge")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read-only operation created .fledge: %v", err)
	}
}

func TestCreateLoadUpdateDeleteLifecycle(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t, root)
	profile := testProfile("review")
	profile.SchemaVersion = 0 // writes treat zero as the current default

	created, err := store.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	if created.SchemaVersion != SchemaVersion {
		t.Fatalf("created schema = %d", created.SchemaVersion)
	}
	path := filepath.Join(root, ".fledge", "profiles", "review.toml")
	assertMode(t, filepath.Join(root, ".fledge"), 0o700)
	assertMode(t, filepath.Dir(path), 0o700)
	assertMode(t, path, 0o600)
	assertMode(t, filepath.Join(filepath.Dir(path), ".profiles.lock"), 0o600)
	dataBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Create(testProfile("review")); err == nil {
		t.Fatal("duplicate Create succeeded")
	} else {
		assertKind(t, err, ErrAlreadyExists, KindAlreadyExists)
	}
	dataAfter, _ := os.ReadFile(path)
	if !reflect.DeepEqual(dataAfter, dataBefore) {
		t.Fatal("duplicate Create changed persisted content")
	}

	loaded, err := store.Load("review")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, created) {
		t.Fatalf("Load() = %#v, want %#v", loaded, created)
	}
	shown, err := store.Show("review")
	if err != nil || !reflect.DeepEqual(shown, loaded) {
		t.Fatalf("Show() = %#v, %v", shown, err)
	}

	loaded.Description = "Updated"
	loaded.NativeArgs = nil
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Update(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if updated.NativeArgs == nil || len(updated.NativeArgs) != 0 {
		t.Fatalf("Update normalized args = %#v", updated.NativeArgs)
	}
	assertMode(t, path, 0o600)
	reloaded, err := store.Load("review")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Description != "Updated" || reloaded.NativeArgs == nil {
		t.Fatalf("updated profile = %#v", reloaded)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("atomic temporary files = %v, %v", matches, err)
	}

	if err := store.Delete("review"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("deleted file still exists: %v", err)
	}
	assertKind(t, store.Delete("review"), ErrNotFound, KindNotFound)
	assertKind(t, loadError(store, "review"), ErrNotFound, KindNotFound)
	_, err = store.Update(reloaded)
	assertKind(t, err, ErrNotFound, KindNotFound)
}

func TestEncodedProfileSizeLimit(t *testing.T) {
	t.Run("oversized create", func(t *testing.T) {
		root := t.TempDir()
		store := newTestStore(t, root)
		profile := profileWithEncodedSize(t, "large", maxProfileSize+1)

		_, err := store.Create(profile)
		assertKind(t, err, ErrInvalid, KindInvalid)
		if _, statErr := os.Lstat(filepath.Join(root, ".fledge")); !errors.Is(statErr, fs.ErrNotExist) {
			t.Fatalf("oversized Create mutated store: %v", statErr)
		}
	})

	t.Run("oversized update is nondestructive", func(t *testing.T) {
		root := t.TempDir()
		store := newTestStore(t, root)
		original := testProfile("review")
		if _, err := store.Create(original); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, ".fledge", "profiles", "review.toml")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		_, err = store.Update(profileWithEncodedSize(t, "review", maxProfileSize+1))
		assertKind(t, err, ErrInvalid, KindInvalid)
		after, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !reflect.DeepEqual(after, before) {
			t.Fatal("oversized Update changed the existing profile")
		}
		loaded, loadErr := store.Load("review")
		if loadErr != nil || !reflect.DeepEqual(loaded, original) {
			t.Fatalf("Load() after rejected update = %#v, %v", loaded, loadErr)
		}
	})

	t.Run("exact boundary", func(t *testing.T) {
		root := t.TempDir()
		store := newTestStore(t, root)
		profile := profileWithEncodedSize(t, "boundary", maxProfileSize)
		if _, err := store.Create(profile); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, ".fledge", "profiles", "boundary.toml")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() != maxProfileSize {
			t.Fatalf("profile size = %d, want %d", info.Size(), maxProfileSize)
		}
		if _, err := store.Load("boundary"); err != nil {
			t.Fatalf("Load exact-boundary profile: %v", err)
		}
	})
}

func TestConcurrentCreateDoesNotOverwrite(t *testing.T) {
	store := newTestStore(t, t.TempDir())
	profiles := []Profile{testProfile("same"), testProfile("same")}
	profiles[0].Description = "first"
	profiles[1].Description = "second"

	var wg sync.WaitGroup
	errs := make(chan error, len(profiles))
	for _, profile := range profiles {
		wg.Add(1)
		go func(p Profile) {
			defer wg.Done()
			_, err := store.Create(p)
			errs <- err
		}(profile)
	}
	wg.Wait()
	close(errs)
	var successes, existing int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAlreadyExists):
			existing++
		default:
			t.Fatalf("Create() error = %v", err)
		}
	}
	if successes != 1 || existing != 1 {
		t.Fatalf("successes = %d, already exists = %d", successes, existing)
	}
	loaded, err := store.Load("same")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Description != "first" && loaded.Description != "second" {
		t.Fatalf("persisted partial/unexpected profile: %#v", loaded)
	}
}

func TestCreateNeverPublishesAMultiplyLinkedProfile(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t, root)
	const attempts = 24
	for index := range attempts {
		name := fmt.Sprintf("profile-%02d", index)
		profile := testProfile(name)
		// Make the pre-publication write long enough for the observer to race
		// with Create. Rename still makes the completed inode visible once.
		profile.Instructions = strings.Repeat("x", 256<<10)
		done := make(chan error, 1)
		go func() {
			_, err := store.Create(profile)
			done <- err
		}()

		path := filepath.Join(root, ".fledge", "profiles", name+".toml")
		for {
			info, err := os.Lstat(path)
			if err == nil {
				if err := validateProfileInode(info); err != nil {
					t.Fatalf("published profile inode is unsafe: %v", err)
				}
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
				info, err := os.Lstat(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := validateProfileInode(info); err != nil {
					t.Fatalf("final profile inode is unsafe: %v", err)
				}
				goto created
			default:
			}
		}
	created:
	}
	matches, err := filepath.Glob(filepath.Join(root, ".fledge", "profiles", ".*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary aliases after Create = %v, %v", matches, err)
	}
}

func TestListSortsProfilesAndIgnoresUnrelatedEntries(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t, root)
	for _, name := range []string{"zeta", "Alpha", "middle"} {
		if _, err := store.Create(testProfile(name)); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(root, ".fledge", "profiles")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("ignore"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "unrelated"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(dir, "unrelated-link")); err != nil {
		t.Fatal(err)
	}

	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(profiles))
	for i := range profiles {
		got[i] = profiles[i].Name
	}
	want := []string{"Alpha", "middle", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List names = %v, want %v", got, want)
	}
}

func TestExistingProfilesDirectoryPermissionsAreNormalized(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".fledge", "profiles")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	store := newTestStore(t, root)
	if _, err := store.List(); err != nil {
		t.Fatal(err)
	}
	assertMode(t, dir, 0o700)
}

func TestUnsafeExistingProfileIsNeverTrustedOrMutated(t *testing.T) {
	t.Run("group and world readable", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, ".fledge", "profiles")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o777); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "unsafe.toml")
		contents := []byte(validTOML(HarnessCodex))
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		store := newTestStore(t, root)

		assertUnsafeProfileOperations(t, store, "unsafe", "permissions")
		assertMode(t, dir, 0o700)
		assertMode(t, path, 0o644)
		got, err := os.ReadFile(path)
		if err != nil || !reflect.DeepEqual(got, contents) {
			t.Fatalf("unsafe profile content changed: %q, %v", got, err)
		}
	})

	t.Run("hard linked", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, ".fledge", "profiles")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, "unrelated.toml")
		contents := []byte(validTOML(HarnessCodex))
		if err := os.WriteFile(target, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "unsafe.toml")
		if err := os.Link(target, path); err != nil {
			t.Fatal(err)
		}
		store := newTestStore(t, root)

		assertUnsafeProfileOperations(t, store, "unsafe", "link count")
		got, err := os.ReadFile(target)
		if err != nil || !reflect.DeepEqual(got, contents) {
			t.Fatalf("hard-linked target changed: %q, %v", got, err)
		}
		targetInfo, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		profileInfo, err := os.Stat(path)
		if err != nil {
			t.Fatalf("unsafe profile was deleted: %v", err)
		}
		if !os.SameFile(targetInfo, profileInfo) {
			t.Fatal("unsafe hard link was replaced")
		}
	})
}

func TestListSurfacesInvalidProfile(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t, root)
	writeRawProfile(t, root, "good", validTOML("codex"))
	writeRawProfile(t, root, "bad", "schema_version = 1\nharness = 4\n")

	_, err := store.List()
	assertKind(t, err, ErrInvalid, KindInvalid)
	var profileErr *Error
	if !errors.As(err, &profileErr) || profileErr.Name != "bad" || profileErr.Op != "list" {
		t.Fatalf("List() error = %#v", err)
	}
}

func TestStrictTOMLAndStructuralValidation(t *testing.T) {
	tests := []struct {
		name string
		toml string
	}{
		{name: "malformed", toml: "schema_version = [\n"},
		{name: "unknown field", toml: validTOML("codex") + "surprise = true\n"},
		{name: "persisted name", toml: validTOML("codex") + `name = "attacker"` + "\n"},
		{name: "duplicate key", toml: validTOML("codex") + `harness = "claude"` + "\n"},
		{name: "malformed type", toml: "schema_version = 1\nharness = [\"codex\"]\n"},
		{name: "trailing invalid content", toml: validTOML("codex") + "this is not valid TOML\n"},
		{name: "missing schema", toml: `harness = "codex"` + "\n"},
		{name: "unsupported schema", toml: "schema_version = 2\nharness = \"codex\"\n"},
		{name: "model without harness", toml: "schema_version = 1\nmodel = \"gpt\"\n"},
		{name: "unsupported harness", toml: validTOML("cursor")},
		{name: "invalid effort", toml: validTOML("codex") + `effort = "extreme"` + "\n"},
		{name: "empty native arg", toml: validTOML("codex") + `native_args = [""]` + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			store := newTestStore(t, root)
			writeRawProfile(t, root, "review", tt.toml)
			_, err := store.Load("review")
			assertKind(t, err, ErrInvalid, KindInvalid)
		})
	}
}

func TestInstructionOnlyProfileRoundTrip(t *testing.T) {
	store := newTestStore(t, t.TempDir())
	want := Profile{
		Name: "orchestrator", SchemaVersion: SchemaVersion,
		Instructions: "Use inherited Fledge only.", NativeArgs: []string{},
	}
	created, err := store.Create(want)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(want.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(created, loaded) || !reflect.DeepEqual(loaded, want) {
		t.Fatalf("round trip = %#v / %#v, want %#v", created, loaded, want)
	}
}

func TestInvalidNamesCannotAddressPaths(t *testing.T) {
	store := newTestStore(t, t.TempDir())
	for _, name := range []string{"", ".", "..", "../escape", "a/b", `a\b`, ".hidden", "two words"} {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			profile := testProfile(name)
			_, err := store.Create(profile)
			assertKind(t, err, ErrInvalid, KindInvalid)
			_, err = store.Load(name)
			assertKind(t, err, ErrInvalid, KindInvalid)
			_, err = store.Update(profile)
			assertKind(t, err, ErrInvalid, KindInvalid)
			assertKind(t, store.Delete(name), ErrInvalid, KindInvalid)
		})
	}
}

func TestInvalidTOMLFilenameIsSurfaced(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t, root)
	writeRawProfile(t, root, "bad name", validTOML("codex"))
	_, err := store.List()
	assertKind(t, err, ErrInvalid, KindInvalid)
}

func TestProfileAndDirectorySymlinksAreRefused(t *testing.T) {
	t.Run("fledge directory", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, ".fledge")); err != nil {
			t.Fatal(err)
		}
		store := newTestStore(t, root)
		_, err := store.Create(testProfile("review"))
		assertKind(t, err, ErrInvalid, KindInvalid)
		if _, err := os.Stat(filepath.Join(outside, "profiles", "review.toml")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("created through symlink: %v", err)
		}
	})

	t.Run("profiles directory", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".fledge"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, ".fledge", "profiles")); err != nil {
			t.Fatal(err)
		}
		store := newTestStore(t, root)
		_, err := store.List()
		assertKind(t, err, ErrInvalid, KindInvalid)
	})

	t.Run("profile file", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		store := newTestStore(t, root)
		dir := filepath.Join(root, ".fledge", "profiles")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(outside, "target.toml")
		original := []byte(validTOML("codex"))
		if err := os.WriteFile(target, original, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "review.toml")); err != nil {
			t.Fatal(err)
		}
		assertKind(t, loadError(store, "review"), ErrInvalid, KindInvalid)
		_, err := store.List()
		assertKind(t, err, ErrInvalid, KindInvalid)
		_, err = store.Create(testProfile("review"))
		assertKind(t, err, ErrInvalid, KindInvalid)
		_, err = store.Update(testProfile("review"))
		assertKind(t, err, ErrInvalid, KindInvalid)
		assertKind(t, store.Delete("review"), ErrInvalid, KindInvalid)
		got, err := os.ReadFile(target)
		if err != nil || !reflect.DeepEqual(got, original) {
			t.Fatalf("symlink target changed: %q, %v", got, err)
		}
	})
}

func TestUnsafeTOMLFileTypesAreRefused(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t, root)
	dir := filepath.Join(root, ".fledge", "profiles")
	if err := os.MkdirAll(filepath.Join(dir, "review.toml"), 0o700); err != nil {
		t.Fatal(err)
	}
	assertKind(t, loadError(store, "review"), ErrInvalid, KindInvalid)
	_, err := store.List()
	assertKind(t, err, ErrInvalid, KindInvalid)
	_, err = store.Update(testProfile("review"))
	assertKind(t, err, ErrInvalid, KindInvalid)
	assertKind(t, store.Delete("review"), ErrInvalid, KindInvalid)
}

func TestSymlinkLockIsRefused(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	store := newTestStore(t, root)
	dir := filepath.Join(root, ".fledge", "profiles")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "lock")
	if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, ".profiles.lock")); err != nil {
		t.Fatal(err)
	}
	_, err := store.Create(testProfile("review"))
	assertKind(t, err, ErrInvalid, KindInvalid)
	got, _ := os.ReadFile(target)
	if string(got) != "untouched" {
		t.Fatalf("lock symlink target changed: %q", got)
	}
}

func TestHardLinkedLockIsRefusedWithoutMutatingTarget(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t, root)
	dir := filepath.Join(root, ".fledge", "profiles")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "unrelated")
	if err := os.WriteFile(target, []byte("untouched"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, filepath.Join(dir, ".profiles.lock")); err != nil {
		t.Fatal(err)
	}

	_, err := store.Create(testProfile("review"))
	assertKind(t, err, ErrInvalid, KindInvalid)
	if !strings.Contains(err.Error(), "hard links") {
		t.Fatalf("error = %v, want hard-link rejection", err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil || string(got) != "untouched" {
		t.Fatalf("hard-link target content = %q, %v", got, readErr)
	}
	assertMode(t, target, 0o644)
	if _, statErr := os.Lstat(filepath.Join(dir, "review.toml")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("profile created despite unsafe lock: %v", statErr)
	}
}

func TestVerifiedProfilesDescriptorSurvivesDirectorySwap(t *testing.T) {
	project, outside := t.TempDir(), t.TempDir()
	store := newTestStore(t, project)
	profiles, exists, err := store.openProfiles(true)
	if err != nil || !exists {
		t.Fatalf("openProfiles() = %v, %v", exists, err)
	}
	defer profiles.Close()

	visible := filepath.Join(project, ".fledge", "profiles")
	parked := filepath.Join(project, ".fledge", "profiles-parked")
	if err := os.Rename(visible, parked); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, visible); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(visible)
		_ = os.Rename(parked, visible)
	})

	profile := testProfile("anchored")
	data, err := encode(profile)
	if err != nil {
		t.Fatal(err)
	}
	err = store.withLock(profiles, func() error {
		return writeReplaceAtomic(profiles, "anchored.toml", data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parked, "anchored.toml")); err != nil {
		t.Fatalf("write did not remain on verified directory inode: %v", err)
	}
	outsideEntries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(outsideEntries) != 0 {
		t.Fatalf("directory swap escaped store: %v", outsideEntries)
	}
}

func TestProjectRootAccessorIsCanonicalAndStable(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t, root)
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.ProjectRoot(); got != want {
		t.Fatalf("ProjectRoot() = %q, want %q", got, want)
	}
}

func TestOversizeProfileIsRefused(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t, root)
	writeRawProfile(t, root, "large", strings.Repeat("#", maxProfileSize+1))
	assertKind(t, loadError(store, "large"), ErrInvalid, KindInvalid)
}

func newTestStore(t *testing.T, root string) *Store {
	t.Helper()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close store: %v", err)
		}
	})
	return store
}

func profileWithEncodedSize(t *testing.T, name string, size int) Profile {
	t.Helper()
	profile := Profile{
		Name:          name,
		SchemaVersion: SchemaVersion,
		Harness:       HarnessCodex,
		NativeArgs:    []string{},
		Instructions:  strings.Repeat("x", size),
	}
	data, err := encodeTOML(profile)
	if err != nil {
		t.Fatal(err)
	}
	overhead := len(data) - size
	if overhead < 0 || size < overhead {
		t.Fatalf("cannot construct encoded profile of %d bytes (overhead %d)", size, overhead)
	}
	profile.Instructions = strings.Repeat("x", size-overhead)
	data, err = encodeTOML(profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != size {
		t.Fatalf("constructed profile size = %d, want %d", len(data), size)
	}
	return profile
}

func validTOML(harness string) string {
	return fmt.Sprintf("schema_version = 1\nharness = %q\n", harness)
}

func writeRawProfile(t *testing.T, root, name, contents string) {
	t.Helper()
	dir := filepath.Join(root, ".fledge", "profiles")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".toml"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode of %s = %o, want %o", path, got, want)
	}
}

func assertKind(t *testing.T, err, sentinel error, kind Kind) {
	t.Helper()
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want errors.Is(%v)", err, sentinel)
	}
	var profileErr *Error
	if !errors.As(err, &profileErr) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if profileErr.Kind != kind {
		t.Fatalf("Error.Kind = %v, want %v", profileErr.Kind, kind)
	}
}

func assertUnsafeProfileOperations(t *testing.T, store *Store, name, reason string) {
	t.Helper()
	updated := testProfile(name)
	updated.Description = "must not replace unsafe inode"
	results := []struct {
		op  string
		err error
	}{
		{op: "list", err: listError(store)},
		{op: "load", err: loadError(store, name)},
		{op: "create", err: createError(store, testProfile(name))},
		{op: "update", err: updateError(store, updated)},
		{op: "delete", err: store.Delete(name)},
	}
	for _, result := range results {
		assertKind(t, result.err, ErrInvalid, KindInvalid)
		if !strings.Contains(result.err.Error(), reason) {
			t.Errorf("%s error = %v, want %q", result.op, result.err, reason)
		}
	}
}

func listError(store *Store) error {
	_, err := store.List()
	return err
}

func createError(store *Store, profile Profile) error {
	_, err := store.Create(profile)
	return err
}

func loadError(store *Store, name string) error {
	_, err := store.Load(name)
	return err
}

func updateError(store *Store, profile Profile) error {
	_, err := store.Update(profile)
	return err
}
