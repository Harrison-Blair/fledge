package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		want  slog.Level
	}{
		{name: "debug", value: "debug", want: slog.LevelDebug},
		{name: "info", value: "info", want: slog.LevelInfo},
		{name: "warn", value: "warn", want: slog.LevelWarn},
		{name: "error", value: "error", want: slog.LevelError},
		{name: "mixed case", value: "DeBuG", want: slog.LevelDebug},
		{name: "padded", value: "  WARN\n", want: slog.LevelWarn},
		{name: "unknown", value: "trace", want: slog.LevelInfo},
		{name: "empty", value: "", want: slog.LevelInfo},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ParseLevel(test.value); got != test.want {
				t.Fatalf("ParseLevel(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestOpenWritesJSONRecords(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "logs", "session")
	logger, closer := mustOpen(t, directory, slog.LevelInfo)
	logger.Info("session started", "session", "fledge-demo", "panes", 2)
	logger.Error("spawn failed", "agent", "alice")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}

	records := readRecords(t, directory)
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}
	for _, key := range []string{"time", "level", "msg"} {
		if _, ok := records[0][key]; !ok {
			t.Fatalf("record %v is missing %q", records[0], key)
		}
	}
	if records[0]["level"] != "INFO" || records[0]["msg"] != "session started" {
		t.Fatalf("first record = %v, want INFO session started", records[0])
	}
	if records[0]["session"] != "fledge-demo" || records[0]["panes"] != float64(2) {
		t.Fatalf("first record attrs = %v, want session and panes", records[0])
	}
	if records[1]["level"] != "ERROR" || records[1]["agent"] != "alice" {
		t.Fatalf("second record = %v, want ERROR agent alice", records[1])
	}
}

func TestOpenFiltersByLevel(t *testing.T) {
	tests := []struct {
		name      string
		level     slog.Level
		wantDebug bool
	}{
		{name: "info suppresses debug", level: slog.LevelInfo, wantDebug: false},
		{name: "debug keeps debug", level: slog.LevelDebug, wantDebug: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			logger, closer := mustOpen(t, directory, test.level)
			logger.Debug("herdr step", "step", "SplitPane")
			logger.Info("session started")
			if err := closer.Close(); err != nil {
				t.Fatal(err)
			}

			records := readRecords(t, directory)
			var sawDebug bool
			for _, record := range records {
				if record["level"] == "DEBUG" {
					sawDebug = true
				}
			}
			if sawDebug != test.wantDebug {
				t.Fatalf("debug record present = %v, want %v (records %v)", sawDebug, test.wantDebug, records)
			}
			if len(records) == 0 {
				t.Fatal("info record was dropped")
			}
		})
	}
}

func TestOpenSecuresCreatedPaths(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, ".fledge", "logs", "session")
	_, closer := mustOpen(t, directory, slog.LevelInfo)
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}

	created := []string{
		filepath.Join(root, ".fledge"),
		filepath.Join(root, ".fledge", "logs"),
		directory,
	}
	for _, path := range created {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("directory %q mode = %v, want 0700", path, got)
		}
	}
	info, err := os.Stat(filepath.Join(directory, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("log file mode = %v, want 0600", got)
	}
}

func TestOpenAppendsOnReopen(t *testing.T) {
	directory := t.TempDir()
	first, closer := mustOpen(t, directory, slog.LevelInfo)
	first.Info("first")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}

	second, closer := mustOpen(t, directory, slog.LevelInfo)
	second.Info("second")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}

	records := readRecords(t, directory)
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2 (reopen truncated the log)", len(records))
	}
	if records[0]["msg"] != "first" || records[1]["msg"] != "second" {
		t.Fatalf("records = %v, want first then second", records)
	}
}

func TestOpenRejectsSymlinkedDirectoryComponent(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "elsewhere")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "logs")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if _, _, err := Open(filepath.Join(link, "session"), slog.LevelInfo); err == nil {
		t.Fatal("Open accepted a symlinked directory component")
	} else if !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("Open() error = %v, want a symlink rejection", err)
	}
}

func TestOpenRejectsSymlinkedLogFile(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "session")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "outside.log")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(directory, FileName)); err != nil {
		t.Fatal(err)
	}

	if _, _, err := Open(directory, slog.LevelInfo); err == nil {
		t.Fatal("Open accepted a symlinked log file")
	} else if !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("Open() error = %v, want a symlink rejection", err)
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "unchanged" {
		t.Fatalf("symlink target = %q, want it untouched", contents)
	}
}

func TestOpenRejectsNonDirectoryPath(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "logs")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := Open(filepath.Join(file, "session"), slog.LevelInfo); err == nil {
		t.Fatal("Open accepted a directory path under a regular file")
	}
}

func TestDiscardWritesNothing(t *testing.T) {
	t.Parallel()
	logger := Discard()
	logger.Debug("debug")
	logger.Info("info", "session", "fledge-demo")
	logger.Warn("warn")
	logger.Error("error", "err", os.ErrNotExist)
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if logger.Enabled(context.Background(), level) {
			t.Fatalf("Discard() logger is enabled at %v", level)
		}
	}
}

func mustOpen(t *testing.T, directory string, level slog.Leveler) (*slog.Logger, io.Closer) {
	t.Helper()
	logger, closer, err := Open(directory, level)
	if err != nil {
		t.Fatal(err)
	}
	return logger, closer
}

func readRecords(t *testing.T, directory string) []map[string]any {
	t.Helper()
	file, err := os.Open(filepath.Join(directory, FileName))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var records []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		record := map[string]any{}
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode log line %q: %v", scanner.Text(), err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}
