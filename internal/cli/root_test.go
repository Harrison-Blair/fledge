package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestUsageErrorsUseExitTwoAndJSONEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--json", "agent", "message", "send", "worker"}, bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != 1 || envelope.OK || envelope.Error.Code != "usage_error" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestAttachRejectsJSONBeforeRuntimeAccess(t *testing.T) {
	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{"agent", "attach", "worker", "--json"}, bytes.NewBuffer(nil), &bytes.Buffer{}, &stderr)
	if code != 2 || !bytes.Contains(stderr.Bytes(), []byte("--json cannot")) {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
}

func TestStartJSONRequiresDetachBeforeRuntimeAccess(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	log := filepath.Join(t.TempDir(), "herdr-invocations")
	binary := filepath.Join(t.TempDir(), "herdr-fake")
	script := fmt.Sprintf("#!/bin/sh\nprintf 'invoked\\n' >> %s\nexit 9\n", strconv.Quote(log))
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"start", "--json", "--herdr-bin", binary},
		bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("error is not JSON: %v (%s)", err, stderr.String())
	}
	if envelope.Error.Code != "usage_error" || !strings.Contains(envelope.Error.Message, "--json requires --detach") {
		t.Fatalf("unexpected error: %#v", envelope)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if _, err := os.Stat(log); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Herdr was accessed before validation: %v", err)
	}
}

func TestSessionOverrideIsRejectedBeforeRuntimeAccess(t *testing.T) {
	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{"status", "--session", "external"},
		bytes.NewBuffer(nil), &bytes.Buffer{}, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "unknown flag: --session") {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
}

func TestStopKeepSessionFlagIsRejected(t *testing.T) {
	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{"stop", "--keep-session"},
		bytes.NewBuffer(nil), &bytes.Buffer{}, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "unknown flag: --keep-session") {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
}

func TestUnknownCommandStillHonorsGlobalJSONFlag(t *testing.T) {
	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--json", "nonsense"}, bytes.NewBuffer(nil), &bytes.Buffer{}, &stderr)
	if code != 2 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("error is not JSON: %v (%s)", err, stderr.String())
	}
	if envelope.Error.Code != "usage_error" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
}
