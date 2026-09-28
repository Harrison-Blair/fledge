package memory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func code(err error) string {
	var remote *herdr.Error
	if errors.As(err, &remote) {
		return remote.Code
	}
	return ""
}

func add(t *testing.T, cwd string, m Memory) {
	t.Helper()
	if err := Add(context.Background(), cwd, m, &libagent.Outcome{}); err != nil {
		t.Fatal(err)
	}
}

func TestAddWritesMemoryAndIndexInPrimaryCheckout(t *testing.T) {
	root := identitytest.Repository(t)
	out := libagent.Outcome{}
	if err := Add(context.Background(), root, valid(), &out); err != nil {
		t.Fatal(err)
	}
	add(t, root, Memory{Name: "alpha", Description: "First fact", Type: "user", Body: "x\n"})
	dir := filepath.Join(root, ".fledge", "memories")
	if got := readFile(t, filepath.Join(dir, "herdr-socket.md")); got != string(Format(valid())) {
		t.Fatalf("%q", got)
	}
	want := "- [alpha](alpha.md) — First fact\n- [herdr-socket](herdr-socket.md) — Herdr commands need socket access\n"
	if got := readFile(t, filepath.Join(dir, IndexName)); got != want {
		t.Fatalf("%q", got)
	}
	if !reflect.DeepEqual(out.Effects[len(out.Effects)-2:], []libagent.Effect{{Action: "created", Kind: "memory", Path: filepath.Join(dir, "herdr-socket.md")}, {Action: "updated", Kind: "file", Path: filepath.Join(dir, IndexName)}}) {
		t.Fatalf("%+v", out.Effects)
	}
	// The managed ignore file keeps memories out of Git.
	if b, err := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=all").Output(); err != nil || len(b) != 0 {
		t.Fatalf("%v %q", err, b)
	}
}

func TestAddRejectsInvalidMemoryBeforeWriting(t *testing.T) {
	root := identitytest.Repository(t)
	m := valid()
	m.Type = "note"
	var input *libagent.InputError
	if err := Add(context.Background(), root, m, &libagent.Outcome{}); !errors.As(err, &input) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestAddRejectsExistingNameWithoutOverwriting(t *testing.T) {
	root := identitytest.Repository(t)
	add(t, root, valid())
	m := valid()
	m.Body = "replacement\n"
	err := Add(context.Background(), root, m, &libagent.Outcome{})
	if code(err) != "memory_exists" {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(root, ".fledge", "memories", "herdr-socket.md")); got != string(Format(valid())) {
		t.Fatalf("%q", got)
	}
}

// commit gives root an initial commit so linked worktrees can be added.
func commit(t *testing.T, root string) {
	t.Helper()
	if b, err := exec.Command("git", "-C", root, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-qm", "init", "--allow-empty").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
}

func TestAddFromLinkedWorktreeWritesPrimaryCheckout(t *testing.T) {
	root := identitytest.Repository(t)
	commit(t, root)
	linked := filepath.Join(root, ".fledge", "worktrees", "feat")
	if b, err := exec.Command("git", "-C", root, "worktree", "add", "-q", "-b", "feat", linked).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	add(t, linked, valid())
	if _, err := os.Stat(filepath.Join(root, ".fledge", "memories", "herdr-socket.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(linked, ".fledge")); !os.IsNotExist(err) {
		t.Fatalf("linked checkout got a .fledge: %v", err)
	}
	dir, err := Dir(context.Background(), linked)
	if err != nil || dir != filepath.Join(root, ".fledge", "memories") {
		t.Fatal(dir, err)
	}
}

func TestConcurrentAddsKeepEveryIndexLine(t *testing.T) {
	root := identitytest.Repository(t)
	// Pausing between listing and writing lets an unserialized writer publish
	// a stale listing over a newer one.
	indexStep = func() { time.Sleep(5 * time.Millisecond) }
	t.Cleanup(func() { indexStep = func() {} })
	const n = 24
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Add(context.Background(), root, Memory{Name: fmt.Sprintf("fact-%02d", i), Description: "d", Type: "project", Body: "b"}, &libagent.Outcome{})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSuffix(readFile(t, filepath.Join(root, ".fledge", "memories", IndexName)), "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("%d lines: %q", len(lines), lines)
	}
}

func TestListAndGetReadWithoutCreatingState(t *testing.T) {
	root := identitytest.Repository(t)
	dir, err := Dir(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if ms, err := List(dir); err != nil || len(ms) != 0 || ms == nil {
		t.Fatalf("%#v %v", ms, err)
	}
	if _, err := Get(dir, "herdr-socket"); code(err) != "memory_not_found" {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	add(t, root, valid())
	add(t, root, Memory{Name: "alpha", Description: "First fact", Type: "user", Body: "x\n"})
	ms, err := List(dir)
	if err != nil || len(ms) != 2 || ms[0].Name != "alpha" || ms[1] != valid() {
		t.Fatalf("%+v %v", ms, err)
	}
	if m, err := Get(dir, "herdr-socket"); err != nil || m != valid() {
		t.Fatalf("%+v %v", m, err)
	}
	var input *libagent.InputError
	if _, err := Get(dir, "../x"); !errors.As(err, &input) {
		t.Fatal(err)
	}
}

func TestListRejectsCorruptMemoryFile(t *testing.T) {
	root := identitytest.Repository(t)
	add(t, root, valid())
	dir := filepath.Join(root, ".fledge", "memories")
	if err := os.WriteFile(filepath.Join(dir, "broken.md"), []byte("no frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := List(dir); err == nil || !strings.Contains(err.Error(), "broken.md") {
		t.Fatal(err)
	}
}

func TestRemoveDeletesMemoryAndRegeneratesIndex(t *testing.T) {
	root := identitytest.Repository(t)
	add(t, root, valid())
	add(t, root, Memory{Name: "alpha", Description: "First fact", Type: "user", Body: "x\n"})
	dir := filepath.Join(root, ".fledge", "memories")
	out := libagent.Outcome{}
	if err := Remove(context.Background(), root, "herdr-socket", &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "herdr-socket.md")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, IndexName)); got != "- [alpha](alpha.md) — First fact\n" {
		t.Fatalf("%q", got)
	}
	if !reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "removed", Kind: "memory", Path: filepath.Join(dir, "herdr-socket.md")}, {Action: "updated", Kind: "file", Path: filepath.Join(dir, IndexName)}}) {
		t.Fatalf("%+v", out.Effects)
	}
	if err := Remove(context.Background(), root, "alpha", &libagent.Outcome{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, IndexName)); got != "" {
		t.Fatalf("%q", got)
	}
}

func TestRemoveUnknownNameFails(t *testing.T) {
	root := identitytest.Repository(t)
	if err := Remove(context.Background(), root, "herdr-socket", &libagent.Outcome{}); code(err) != "memory_not_found" {
		t.Fatal(err)
	}
	add(t, root, valid())
	if err := Remove(context.Background(), root, "alpha", &libagent.Outcome{}); code(err) != "memory_not_found" {
		t.Fatal(err)
	}
}
