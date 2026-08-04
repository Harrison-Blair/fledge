package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/logging"
	"github.com/Harrison-Blair/fledge/internal/statedir"
)

func captureSessionLog(manager *Manager) *bytes.Buffer {
	var records bytes.Buffer
	manager.logFactory = func(root, session string) (*slog.Logger, io.Closer, error) {
		return slog.New(slog.NewJSONHandler(&records, &slog.HandlerOptions{Level: slog.LevelDebug})), io.NopCloser(nil), nil
	}
	return &records
}

func decodeLogRecords(t *testing.T, records *bytes.Buffer) []map[string]any {
	t.Helper()
	var decoded []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(records.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		for _, key := range []string{"time", "level", "msg"} {
			if _, present := record[key]; !present {
				t.Fatalf("log line %q is missing %q", line, key)
			}
		}
		decoded = append(decoded, record)
	}
	return decoded
}

func hasLogRecord(records []map[string]any, level, message string) bool {
	for _, record := range records {
		if record["level"] == level && record["msg"] == message {
			return true
		}
	}
	return false
}

func startedTestManager(t *testing.T) (*Manager, *fakeHerdr, string) {
	t.Helper()
	root := t.TempDir()
	initTestProject(t, root)
	client := &fakeHerdr{snapshot: testSnapshot()}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	return manager, client, root
}

func startOptionsForLogging() StartOptions {
	return StartOptions{
		Harness: "codex", HarnessSet: true, Model: "custom/model", ModelSet: true,
		Timeout: 45 * time.Second,
	}
}

func TestStartWritesSessionDebugLog(t *testing.T) {
	t.Parallel()
	manager, _, root := startedTestManager(t)
	records := captureSessionLog(manager)

	if err := manager.Start(context.Background(), root, startOptionsForLogging()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	decoded := decodeLogRecords(t, records)
	for _, want := range []struct{ level, message string }{
		{"INFO", "start invoked"},
		{"DEBUG", "herdr server started"},
		{"DEBUG", "session messaging initialized"},
		{"DEBUG", "orchestrator layout selected"},
		{"DEBUG", "orchestrator agent started"},
		{"INFO", "orchestrator started"},
	} {
		if !hasLogRecord(decoded, want.level, want.message) {
			t.Errorf("session log is missing %s %q; records = %v", want.level, want.message, decoded)
		}
	}
}

func TestFailedStartLogsRollback(t *testing.T) {
	t.Parallel()
	manager, client, root := startedTestManager(t)
	client.waitErr = errors.New("herdr never became ready")
	records := captureSessionLog(manager)

	if err := manager.Start(context.Background(), root, startOptionsForLogging()); err == nil {
		t.Fatal("Start() succeeded, want failure")
	}
	decoded := decodeLogRecords(t, records)
	if !hasLogRecord(decoded, "ERROR", "orchestrator start failed") {
		t.Errorf("session log is missing the start failure; records = %v", decoded)
	}
	if !hasLogRecord(decoded, "WARN", "rolling back failed start") {
		t.Errorf("session log is missing the rollback warning; records = %v", decoded)
	}
}

func TestSendMessageLogsMetadataWithoutBody(t *testing.T) {
	t.Parallel()
	manager, _, root := newMessagingManager(t)
	records := captureSessionLog(manager)
	body := "confidential-payload-must-not-leak"

	message, err := manager.SendMessage(context.Background(), root, "worker", body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(records.String(), body) {
		t.Fatalf("session log leaked the message body: %q", records.String())
	}
	decoded := decodeLogRecords(t, records)
	for _, want := range []struct{ level, message string }{
		{"INFO", "message created"},
		{"DEBUG", "delivery attempt"},
		{"DEBUG", "delivered"},
	} {
		if !hasLogRecord(decoded, want.level, want.message) {
			t.Errorf("session log is missing %s %q; records = %v", want.level, want.message, decoded)
		}
	}
	for _, record := range decoded {
		if record["msg"] != "message created" {
			continue
		}
		if record["message_id"] != message.ID || record["body_bytes"] != float64(len(body)) {
			t.Errorf("message created record = %v", record)
		}
	}
}

func TestSessionLogOpenFailureDoesNotBreakCommands(t *testing.T) {
	t.Parallel()
	manager, _, root := newMessagingManager(t)
	var output bytes.Buffer
	manager.output = &output
	manager.logFactory = func(root, session string) (*slog.Logger, io.Closer, error) {
		return nil, nil, errors.New("log directory is unwritable")
	}

	if _, err := manager.SendMessage(context.Background(), root, "worker", "still delivered"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if !strings.Contains(output.String(), "session debug log unavailable") {
		t.Errorf("output = %q, want a session debug log warning", output.String())
	}
}

func TestStartCreatesSessionDebugLogFile(t *testing.T) {
	t.Parallel()
	manager, _, root := startedTestManager(t)

	if err := manager.Start(context.Background(), root, startOptionsForLogging()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	value, found, err := readRecord(root)
	if err != nil || !found {
		t.Fatalf("readRecord() = %#v, %v, %v", value, found, err)
	}
	logPath := filepath.Join(statedir.Session(root, value.SessionName), logging.FileName)
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("session debug log missing: %v", err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Errorf("session debug log permissions = %o, want 600", permissions)
	}
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "start invoked") {
		t.Errorf("session debug log = %q, want a start record", contents)
	}
}
