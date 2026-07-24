package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/contextdoc"
	"github.com/Harrison-Blair/fledge/internal/workspace"
)

// scanWorkspace scaffolds a workspace with files at the root and under the
// nested subdirectory, and appends "secret.txt" to its .fledgeignore.
func scanWorkspace(t *testing.T) (root, sub string) {
	t.Helper()
	root, sub = scaffoldedWorkspace(t)
	for _, p := range []string{"keep.txt", "secret.txt", "sub/deep/inner.txt", "sub/deep/secret.txt"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(p)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	appendIgnore(t, root, "secret.txt\n")
	return root, sub
}

func appendIgnore(t *testing.T, root, lines string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(root, ".fledge", ".fledgeignore"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(lines); err != nil {
		t.Fatal(err)
	}
}

// scanJSON runs `context scan [dir] --json` and returns the decoded result.
func scanJSON(t *testing.T, args ...string) (string, []string) {
	t.Helper()
	out, err := captureRun(t, append([]string{"context", "scan"}, append(args, "--json")...)...)
	if err != nil {
		t.Fatalf("context scan: %v", err)
	}
	var got contextdoc.Scan
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if got.SchemaVersion != contextdoc.SchemaVersion || got.FileCount != len(got.Files) {
		t.Fatalf("scan contract = %+v", got)
	}
	var total int64
	paths := make([]string, len(got.Files))
	for i, f := range got.Files {
		paths[i] = f.Path
		total += f.Size
	}
	if got.TotalSize != total {
		t.Fatalf("total_size = %d, want derived %d", got.TotalSize, total)
	}
	return got.Root, paths
}

func assertPaths(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("paths = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paths = %q, want %q", got, want)
		}
	}
}

// Run from a nested subdirectory, scan resolves the workspace git-style and
// lists the whole tree with the root ignore file applied.
func TestContextScanWalksUpFromSubdir(t *testing.T) {
	root, sub := scanWorkspace(t)
	t.Chdir(sub)
	gotRoot, paths := scanJSON(t)
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != wantRoot {
		t.Fatalf("root = %q, want %q", gotRoot, wantRoot)
	}
	assertPaths(t, paths, []string{"keep.txt", "sub/deep/inner.txt"})
}

// A dir argument below the root limits the listing to that subtree; paths stay
// root-relative and root ignore patterns still apply.
func TestContextScanDirArgLimitsToSubtree(t *testing.T) {
	root, _ := scanWorkspace(t)
	t.Chdir(root)
	_, paths := scanJSON(t, "sub")
	assertPaths(t, paths, []string{"sub/deep/inner.txt"})
}

// A subtree that the ignore file excludes lists nothing, matching the walk's
// pruning: nothing beneath an excluded directory is reachable.
func TestContextScanIgnoredSubtreeIsEmpty(t *testing.T) {
	root, _ := scanWorkspace(t)
	appendIgnore(t, root, "sub/\n")
	t.Chdir(root)
	_, paths := scanJSON(t, "sub")
	assertPaths(t, paths, nil)
	out, err := captureRun(t, "context", "scan", "sub", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"files": []`) {
		t.Fatalf("empty scan files must be an array: %s", out)
	}
}

// Outside any workspace the scan fails like other commands do, instead of
// silently scanning with an empty matcher.
func TestContextScanOutsideWorkspaceErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := captureRun(t, "context", "scan")
	if !errors.Is(err, workspace.ErrNotFound) {
		t.Fatalf("err = %v, want workspace.ErrNotFound", err)
	}
}
