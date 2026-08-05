package agentcontext

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func sampleReport() Report {
	used := 1000
	window := 200000
	percent := 0.5
	observed := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	reason := ReasonAwaitingFirstResponse
	return Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
		Agents: []AgentContext{
			{Name: "orchestrator", Harness: "claude", Revision: 3, Status: StatusAvailable, Used: &used, Window: &window, Percent: &percent, ObservedAt: &observed},
			{Name: "worker", Harness: "pi", Revision: 1, Status: StatusUnknown, Reason: &reason},
		},
	}
}

func TestPersistWritesPrivateFileUnderPrivateDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "context")
	if err := Persist(dir, sampleReport()); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %o, want 0700", dirInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(filepath.Join(dir, reportFile))
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %o, want 0600", fileInfo.Mode().Perm())
	}
}

func TestPersistSecuresExistingContextDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced on Windows")
	}
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "context")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Persist(dir, sampleReport()); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("existing dir mode = %o, want 0700", info.Mode().Perm())
	}
}

func TestPersistLoadRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	want := sampleReport()
	if err := Persist(dir, want); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	got, ok, err := Load(dir)
	if err != nil || !ok {
		t.Fatalf("Load() = ok %v err %v", ok, err)
	}
	if got.SchemaVersion != want.SchemaVersion || len(got.Agents) != len(want.Agents) {
		t.Fatalf("round trip mismatch: got %+v", got)
	}
	if got.Agents[0].Used == nil || *got.Agents[0].Used != 1000 {
		t.Errorf("used not preserved: %+v", got.Agents[0])
	}
	if got.Agents[1].Reason == nil || *got.Agents[1].Reason != ReasonAwaitingFirstResponse {
		t.Errorf("reason not preserved: %+v", got.Agents[1])
	}
}

func TestPersistOverwritesAtomically(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := Persist(dir, sampleReport()); err != nil {
		t.Fatal(err)
	}
	second := sampleReport()
	second.Agents = second.Agents[:1]
	if err := Persist(dir, second); err != nil {
		t.Fatal(err)
	}
	got, _, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Agents) != 1 {
		t.Errorf("len(Agents) = %d after overwrite, want 1", len(got.Agents))
	}
	// No temporary files must linger in the directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != reportFile {
		t.Errorf("directory entries = %v, want only %q", entries, reportFile)
	}
}

func TestLoadMissingIsNotAnError(t *testing.T) {
	t.Parallel()
	_, ok, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for a missing report", err)
	}
	if ok {
		t.Error("Load() ok = true, want false when no report exists")
	}
}

func TestCleanupRemovesDirectory(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "context")
	if err := Persist(dir, sampleReport()); err != nil {
		t.Fatal(err)
	}
	if err := Cleanup(dir); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("context directory still present after Cleanup: %v", err)
	}
	// Cleanup of an already-absent directory is a no-op.
	if err := Cleanup(dir); err != nil {
		t.Errorf("Cleanup() of missing dir error = %v, want nil", err)
	}
}

func TestPersistRejectsSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, reportFile)); err != nil {
		t.Fatal(err)
	}
	if err := Persist(dir, sampleReport()); err == nil {
		t.Fatal("Persist() error = nil, want rejection of a symlinked report path")
	}
}

func TestPersistRejectsSymlinkDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "context")
	if err := os.Symlink(target, dir); err != nil {
		t.Fatal(err)
	}
	if err := Persist(dir, sampleReport()); err == nil {
		t.Fatal("Persist() error = nil, want rejection of a symlinked context directory")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink target was modified: %v", entries)
	}
}
