package scan

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/ignore"
)

// tree writes each path relative to a fresh temp dir, creating parents.
func tree(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// files runs a scan and returns just the paths, for tests about what is listed
// rather than what is reported about each entry.
func files(t *testing.T, root, patterns string) []string {
	t.Helper()
	m, err := ignore.Parse(strings.NewReader(patterns), root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Files(root, m)
	if err != nil {
		t.Fatal(err)
	}

	paths := make([]string, len(got))
	for i, f := range got {
		paths[i] = f.Path
	}
	return paths
}

func TestFilesNoPatterns(t *testing.T) {
	root := tree(t, "go.mod", "cmd/fledge/main.go", "internal/scan/scan.go")

	want := []string{"cmd/fledge/main.go", "go.mod", "internal/scan/scan.go"}
	if got := files(t, root, ""); !reflect.DeepEqual(got, want) {
		t.Errorf("Files() = %v, want %v", got, want)
	}
}

func TestFilesReportsSize(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "empty.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := ignore.Parse(strings.NewReader(""), root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Files(root, m)
	if err != nil {
		t.Fatal(err)
	}

	want := []File{{Path: "a.txt", Size: 5}, {Path: "empty.txt", Size: 0}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Files() = %v, want %v", got, want)
	}
}

func TestFilesPrunesIgnoredDir(t *testing.T) {
	root := tree(t, "go.mod", ".fledge/.fledgeignore", ".git/config", ".git/objects/ab/cd")

	want := []string{"go.mod"}
	if got := files(t, root, ".fledge/\n.git/\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("Files() = %v, want %v", got, want)
	}
}

func TestFilesIgnoresSingleFile(t *testing.T) {
	root := tree(t, "a.log", "keep.log", "main.go")

	want := []string{"keep.log", "main.go"}
	if got := files(t, root, "*.log\n!keep.log\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("Files() = %v, want %v", got, want)
	}
}

// Git cannot re-include a file whose parent directory is excluded; pruning the
// directory is what makes fledge match that behavior.
func TestFilesCannotReincludeUnderPrunedDir(t *testing.T) {
	root := tree(t, "build/keep.go", "main.go")

	want := []string{"main.go"}
	if got := files(t, root, "build/\n!build/keep.go\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("Files() = %v, want %v", got, want)
	}
}
