package checks_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/doctor/checks"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/models"
)

type fakeInfo struct{ mode fs.FileMode }

func (fakeInfo) Name() string        { return "s" }
func (fakeInfo) Size() int64         { return 0 }
func (f fakeInfo) Mode() fs.FileMode { return f.mode }
func (fakeInfo) ModTime() time.Time  { return time.Time{} }
func (fakeInfo) IsDir() bool         { return false }
func (fakeInfo) Sys() any            { return nil }

// integrationList builds a result whose listed targets are marked available.
func integrationList(available ...string) herdr.IntegrationListResult {
	set := map[string]bool{}
	for _, t := range available {
		set[t] = true
	}
	var infos []herdr.IntegrationInfo
	for _, kind := range append(models.ModelHarnesses(), "grok") {
		command := kind
		if kind == "cursor" {
			command = "cursor-agent"
		}
		state := "not_installed"
		if set[kind] {
			state = "current"
		}
		infos = append(infos, herdr.IntegrationInfo{Target: kind, Label: kind, Command: command, Available: set[kind], State: state})
	}
	return herdr.IntegrationListResult{Type: "integration_list", Integrations: infos}
}

// u32 returns a pointer to a uint32 for optional capability fields.
func u32(v uint32) *uint32 { return &v }

func pong(protocol uint32) herdr.PongResult {
	return herdr.PongResult{Type: "pong", Version: "0.9.1", Protocol: protocol, Capabilities: &herdr.Capabilities{LiveHandoff: true, HealthCheck: true}}
}

// discovery points at an empty home; its Run errors so any accidental command
// execution by doctor is caught. Doctor must only read local cache files.
func discovery() models.Discovery {
	return models.Discovery{Home: "/nonexistent-home-fledge", Run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("doctor must not execute harness commands")
	}}
}

// writeCache writes a model cache fixture file.
func writeCache(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// homeWithPi returns a home whose pi model store holds the given JSON.
func homeWithPi(t *testing.T, store string) string {
	t.Helper()
	home := t.TempDir()
	writeCache(t, filepath.Join(home, ".pi", "agent", "models-store.json"), store)
	return home
}

// fatalRunDiscovery fails the test if doctor ever executes a command.
func fatalRunDiscovery(t *testing.T, home string) models.Discovery {
	return models.Discovery{Home: home, Run: func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("doctor must not execute harness commands")
		return nil, nil
	}}
}

func env(vars map[string]string, mode fs.FileMode, statErr error) checks.Environment {
	return checks.Environment{
		Getenv: func(k string) string { return vars[k] },
		Stat: func(string) (fs.FileInfo, error) {
			if statErr != nil {
				return nil, statErr
			}
			return fakeInfo{mode: mode}, nil
		},
		Getwd: func() (string, error) { return "/work/dir", nil },
	}
}

func healthyEnv() checks.Environment {
	return env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/run/herdr.sock", "HERDR_PANE_ID": "w1:p2", "HERDR_SESSION": "main"}, fs.ModeSocket|0o600, nil)
}
