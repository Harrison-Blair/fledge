package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

// mark creates a .fledge directory under dir.
func mark(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, scaffold.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestFindRootWalksUpFromNestedSubdirectory(t *testing.T) {
	root := t.TempDir()
	mark(t, root)
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := FindRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("FindRoot(%q) = %q, want %q", sub, got, want)
	}
}

func TestFindRootAtRootItself(t *testing.T) {
	root := t.TempDir()
	mark(t, root)

	got, err := FindRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(root)
	if got != want {
		t.Errorf("FindRoot(root) = %q, want %q", got, want)
	}
}

// A nested workspace shadows the enclosing one for everything beneath it.
func TestFindRootPrefersNearestAncestor(t *testing.T) {
	outer := t.TempDir()
	mark(t, outer)
	inner := filepath.Join(outer, "vendored")
	mark(t, inner)
	sub := filepath.Join(inner, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := FindRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(inner)
	if got != want {
		t.Errorf("FindRoot = %q, want nearest workspace %q", got, want)
	}
}

func TestFindRootWithoutWorkspaceIsHardError(t *testing.T) {
	_, err := FindRoot(t.TempDir())
	if err == nil {
		t.Fatal("FindRoot found a workspace in a bare temp dir")
	}
	if !strings.Contains(err.Error(), "run fledge init") {
		t.Errorf("error %q does not point at fledge init", err)
	}
}

// A stray .fledge regular file does not mark a workspace.
func TestFindRootSkipsFledgeRegularFile(t *testing.T) {
	root := t.TempDir()
	mark(t, root)
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, scaffold.DirName), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := FindRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(root)
	if got != want {
		t.Errorf("FindRoot = %q, want %q (the file should be skipped)", got, want)
	}
}

// Reaching one workspace through a symlink must yield the same identity as
// reaching it directly, or a daemon and a client would disagree on the hash.
func TestFindRootCanonicalizesSymlinkedPath(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "real")
	mark(t, root)
	link := filepath.Join(base, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}

	direct, err := FindRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	viaLink, err := FindRoot(link)
	if err != nil {
		t.Fatal(err)
	}
	if direct != viaLink {
		t.Errorf("FindRoot via symlink = %q, direct = %q", viaLink, direct)
	}
}

func TestHashStableAndDiffersPerWorkspace(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	if Hash(a) != Hash(a) {
		t.Error("Hash is not stable for one path")
	}
	if Hash(a) == Hash(b) {
		t.Errorf("distinct workspaces share hash %q", Hash(a))
	}
	if n := len(Hash(a)); n != 12 {
		t.Errorf("Hash length = %d, want 12", n)
	}
}

func TestSlugSanitization(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"myproject", "myproject"},
		{"MyProject", "myproject"},
		{"my.proj", "my-proj"},
		{"über", "ber"},
		{"a--b_c", "a-b-c"},
		{"aaaaaaaaaaaaaaaaaaaa", "aaaaaaaaaaaaaaaa"}, // 20 chars -> 16
		{"aaaaaaaaaaaaaaa.b", "aaaaaaaaaaaaaaa"},     // truncation leaves no trailing dash
		{"...", "ws"},
	}
	for _, test := range tests {
		root := filepath.Join(t.TempDir(), test.base)
		got := Slug(root)
		wantPrefix := test.want + "-"
		if !strings.HasPrefix(got, wantPrefix) {
			t.Errorf("Slug(...%q) = %q, want prefix %q", test.base, got, wantPrefix)
			continue
		}
		if suffix := strings.TrimPrefix(got, wantPrefix); len(suffix) != 6 {
			t.Errorf("Slug(...%q) = %q: hash suffix %q is not 6 chars", test.base, got, suffix)
		}
	}
}
