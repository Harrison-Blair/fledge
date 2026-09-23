package checks_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/doctor/checks"
	"github.com/Harrison-Blair/fledge/internal/doctor/report"
)

func TestConfigurationHealthy(t *testing.T) {
	c := checks.Configuration(healthyEnv())
	if c.Name != "configuration" || c.Status != report.OK {
		t.Fatalf("configuration %+v", c)
	}
	if c.Detail != "HERDR_ENV=1 socket=/run/herdr.sock (0600) pane=w1:p2 session=main cwd=/work/dir" {
		t.Fatalf("detail = %q", c.Detail)
	}
	want := report.ConfigData{HerdrEnv: "1", SocketPath: "/run/herdr.sock", SocketMode: "0600", PaneID: "w1:p2", Session: "main", Cwd: "/work/dir", SocketField: "/run/herdr.sock (0600)"}
	if c.Data != want {
		t.Fatalf("data = %#v, want %#v", c.Data, want)
	}
}

func TestConfigurationSocketModeWarn(t *testing.T) {
	c := checks.Configuration(env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/run/herdr.sock", "HERDR_PANE_ID": "w1:p2"}, fs.ModeSocket|0o644, nil))
	if c.Status != report.Warn || !strings.Contains(c.Detail, "644") {
		t.Fatalf("configuration %+v", c)
	}
}

func TestConfigurationMissingSocketFails(t *testing.T) {
	c := checks.Configuration(env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/gone.sock", "HERDR_PANE_ID": "w1:p2"}, 0, fs.ErrNotExist))
	if c.Status != report.Fail || !strings.Contains(c.Detail, "missing") {
		t.Fatalf("configuration %+v", c)
	}
}

func TestConfigurationStatErrorWarns(t *testing.T) {
	// A non-not-exist stat error leaves the mode unknown: warn, not fail.
	c := checks.Configuration(env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/run/herdr.sock", "HERDR_PANE_ID": "w1:p2"}, 0, errors.New("permission denied")))
	if c.Status != report.Warn || !strings.Contains(c.Detail, "permission denied") {
		t.Fatalf("configuration %+v", c)
	}
}

func TestConfigurationNotASocketFails(t *testing.T) {
	c := checks.Configuration(env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/etc/hosts", "HERDR_PANE_ID": "w1:p2"}, 0o600, nil))
	if c.Status != report.Fail || !strings.Contains(c.Detail, "socket") {
		t.Fatalf("configuration %+v", c)
	}
}

func TestConfigurationOutsideHerdrFails(t *testing.T) {
	c := checks.Configuration(env(map[string]string{}, 0, errors.New("unset")))
	if c.Status != report.Fail {
		t.Fatal("configuration should fail outside Herdr")
	}
	if c.Detail != "HERDR_ENV=- socket=unset pane=- session=- cwd=/work/dir" {
		t.Fatalf("detail = %q", c.Detail)
	}
}

func TestConfigurationPaneAbsentIsWarn(t *testing.T) {
	c := checks.Configuration(env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/run/herdr.sock"}, fs.ModeSocket|0o600, nil))
	if c.Status != report.Warn {
		t.Fatalf("configuration %+v", c)
	}
	if !strings.Contains(c.Detail, "cwd=/work/dir") {
		t.Fatalf("detail should include cwd: %+v", c)
	}
}

func TestLocalEnvironmentUsesProcessState(t *testing.T) {
	t.Setenv("FLEDGE_CHECKS_PROBE", "set")
	e := checks.LocalEnvironment()
	if e.Getenv("FLEDGE_CHECKS_PROBE") != "set" || e.Stat == nil || e.Getwd == nil {
		t.Fatal("local environment does not read the process")
	}
}
