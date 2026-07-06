package scan

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return root
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRun(t *testing.T) {
	root := initRepo(t)
	write(t, root, "README.md", "hello")            // 5 bytes, <root>
	write(t, root, "dir1/a.txt", "aaaa")            // 4 bytes
	write(t, root, "dir1/b.txt", "bb")              // 2 bytes
	write(t, root, "dir2/c.txt", "c")               // 1 byte
	write(t, root, "dir3/skip.log", "ignored")      // filtered by scan-ignore
	write(t, root, ".fledge/scan-ignore", "*.log\n.fledge/\n")

	res, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	if res.ShortCommit != "none" {
		t.Errorf("no commits yet: ShortCommit = %q, want none", res.ShortCommit)
	}
	names := make([]string, len(res.Modules))
	for i, m := range res.Modules {
		names[i] = m.Name
	}
	want := []string{"<root>", "dir1", "dir2"}
	if len(names) != 3 || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] {
		t.Fatalf("modules = %v, want %v", names, want)
	}
	dir1 := res.Modules[1]
	if dir1.Count != 2 || dir1.Bytes != 6 {
		t.Errorf("dir1 = %+v, want count 2, bytes 6", dir1)
	}
	if len(dir1.Files) != 2 || dir1.Files[0] != "dir1/a.txt" || dir1.Files[1] != "dir1/b.txt" {
		t.Errorf("dir1 files = %v", dir1.Files)
	}
	rootMod := res.Modules[0]
	if rootMod.Count != 1 || rootMod.Files[0] != "README.md" || rootMod.Bytes != 5 {
		t.Errorf("<root> = %+v", rootMod)
	}
}

func TestRunNoScanIgnore(t *testing.T) {
	root := initRepo(t)
	write(t, root, "a.md", "x")
	res, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Modules) != 1 || res.Modules[0].Count != 1 {
		t.Errorf("modules = %+v", res.Modules)
	}
}

func TestRunEmptyRepo(t *testing.T) {
	root := initRepo(t)
	res, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Modules) != 0 {
		t.Errorf("want no modules, got %+v", res.Modules)
	}
}
