package agentcontext

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
)

// reportFile is the fixed name of the persisted report inside the context
// directory.
const reportFile = "report.json"

// Persist writes report to dir/report.json atomically. The directory is created
// (and kept) at 0700 and the file lands at 0600, so no other user can read the
// token counts. An interrupted write leaves the previous report intact.
func Persist(dir string, report Report) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create context directory %q: %w", dir, err)
	}
	if err := fsutil.RejectSymlink(dir); err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("inspect context directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("context path %q is not a directory", dir)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure context directory %q: %w", dir, err)
	}
	contents, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode context report: %w", err)
	}
	contents = append(contents, '\n')

	path := filepath.Join(dir, reportFile)
	if err := fsutil.RejectSymlink(path); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, reportFile+".*")
	if err != nil {
		return fmt.Errorf("create temporary context report in %q: %w", dir, err)
	}
	tempPath := temporary.Name()
	if err := writeReport(temporary, contents); err != nil {
		_ = temporary.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temporary context report %q: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace context report %q: %w", path, err)
	}
	return fsutil.SyncDirectory(dir)
}

func writeReport(file *os.File, contents []byte) error {
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("set context report permissions: %w", err)
	}
	if _, err := file.Write(contents); err != nil {
		return fmt.Errorf("write context report: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync context report: %w", err)
	}
	return nil
}

// Load reads a persisted report. The boolean is false when no report has been
// written yet, which callers treat as an empty, non-error state.
func Load(dir string) (Report, bool, error) {
	path := filepath.Join(dir, reportFile)
	file, err := fsutil.OpenRegular(path, os.O_RDONLY, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Report{}, false, nil
		}
		return Report{}, false, fmt.Errorf("open context report %q: %w", path, err)
	}
	defer file.Close()

	var report Report
	if err := json.NewDecoder(file).Decode(&report); err != nil {
		return Report{}, false, fmt.Errorf("decode context report %q: %w", path, err)
	}
	return report, true, nil
}

// Cleanup removes the context directory and everything under it. It is safe to
// call when the directory was never created.
func Cleanup(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove context directory %q: %w", dir, err)
	}
	return nil
}
