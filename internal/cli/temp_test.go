package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdrtest"
	"github.com/Harrison-Blair/fledge/internal/project"
)

func TestTempCleanHumanAndJSONAreRepeatable(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	binary := herdrtest.WriteBinary(t, t.TempDir(), herdrtest.Options{
		Version:  herdrtest.VersionOutput,
		Sessions: []herdrtest.SessionCase{{Payload: `{"sessions":[]}`}},
	})
	tempDir := project.TempDir(root)
	ignorePath := filepath.Join(root, ".fledge", ".gitignore")
	if err := os.WriteFile(ignorePath, []byte("/keep/\n/logs/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(tempDir, "nested", "stale")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("discard"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"temp", "clean", "--herdr-bin", binary},
		bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Cleaned project temp directory\n") ||
		!strings.Contains(stdout.String(), "Project: "+root+"\n") ||
		!strings.Contains(stdout.String(), "Temp: "+tempDir+"\n") {
		t.Fatalf("unexpected human output: %s", stdout.String())
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale temp file remains: %v", err)
	}
	info, err := os.Stat(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("temp directory mode = %v", info.Mode())
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute(context.Background(), []string{"--json", "temp", "clean", "--herdr-bin", binary},
		bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("repeat exit=%d stderr=%s", code, stderr.String())
	}
	var envelope struct {
		SchemaVersion int `json:"schema_version"`
		OK            bool
		Data          struct {
			ProjectRoot string `json:"project_root"`
			TempDir     string `json:"temp_dir"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != 1 || !envelope.OK || envelope.Data.ProjectRoot != root ||
		envelope.Data.TempDir != tempDir {
		t.Fatalf("unexpected JSON envelope: %#v", envelope)
	}
	ignore, err := os.ReadFile(ignorePath)
	if err != nil || string(ignore) != "/keep/\n/logs/\n/tmp/\n" {
		t.Fatalf("runtime ignore was not updated: %q, %v", ignore, err)
	}
}

func TestTempCleanRefusesRunningSessionWithoutChangingFiles(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	tempDir := project.TempDir(root)
	if err := os.Mkdir(tempDir, 0o700); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(tempDir, "keep")
	if err := os.WriteFile(kept, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary, _, _ := fakeStartBinary(t, root, project.SessionName(root), startOptions{Running: true})

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--json", "temp", "clean", "--herdr-bin", binary},
		bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "temp_in_use" {
		t.Fatalf("unexpected error: %#v", envelope)
	}
	if data, err := os.ReadFile(kept); err != nil || string(data) != "preserve" {
		t.Fatalf("running-session temp content changed: data=%q err=%v", data, err)
	}
}

func TestTempCleanReportsFilesystemFailure(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	ignore := filepath.Join(root, ".fledge", ".gitignore")
	if err := os.Remove(ignore); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(ignore, 0o700); err != nil {
		t.Fatal(err)
	}
	binary := herdrtest.WriteBinary(t, t.TempDir(), herdrtest.Options{
		Version:  herdrtest.VersionOutput,
		Sessions: []herdrtest.SessionCase{{Payload: `{"sessions":[]}`}},
	})

	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{"temp", "clean", "--herdr-bin", binary},
		bytes.NewBuffer(nil), &bytes.Buffer{}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "Error [temp_clean_failed]") {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
}

func TestTempCleanRejectsArguments(t *testing.T) {
	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{"temp", "clean", "unexpected"},
		bytes.NewBuffer(nil), &bytes.Buffer{}, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "accepts no arguments") {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
}
