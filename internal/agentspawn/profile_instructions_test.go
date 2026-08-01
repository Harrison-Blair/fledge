package agentspawn

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/project"
)

func TestMaterializeProfileInstructionsIsDeterministicExactAtomicAndPrivate(t *testing.T) {
	root := t.TempDir()
	instructions := "Review exactly.\nDo not trim this trailing space. \n"

	first, err := MaterializeProfileInstructions(root, instructions)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MaterializeProfileInstructions(root, instructions)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !filepath.IsAbs(first) {
		t.Fatalf("materialized paths = %q and %q", first, second)
	}
	sum := sha256.Sum256([]byte(instructions))
	wantPath := filepath.Join(project.TempDir(root), profileInstructionsDir, fmt.Sprintf("%x.txt", sum))
	if first != wantPath {
		t.Fatalf("path = %q, want %q", first, wantPath)
	}
	contents, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contents, []byte(instructions)) {
		t.Fatalf("contents = %q, want %q", contents, instructions)
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file permissions = %04o, want 0600", got)
	}
	for _, dir := range []string{project.TempDir(root), filepath.Dir(first)} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("directory %s permissions = %04o, want 0700", dir, got)
		}
	}
	temporary, err := filepath.Glob(filepath.Join(filepath.Dir(first), ".*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporary) != 0 {
		t.Fatalf("atomic temporary files remain: %v", temporary)
	}
}

func TestMaterializeProfileInstructionsStoresPathLikeTextAsContent(t *testing.T) {
	root := t.TempDir()
	path, err := MaterializeProfileInstructions(root, "AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "AGENTS.md" || filepath.Base(path) == "AGENTS.md" {
		t.Fatalf("path/content = %q / %q", path, contents)
	}
}
