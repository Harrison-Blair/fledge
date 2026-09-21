package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDoctorHelp(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"doctor", "--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"doctor", "Diagnose", "--json", "--verbose"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
}

func TestDoctorRejectsArgs(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"doctor", "extra"}, &out); err == nil {
		t.Fatal("doctor accepted a positional argument")
	}
}

func TestDoctorRejectsUnknownFlag(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"doctor", "--nope"}, &out); err == nil {
		t.Fatal("doctor accepted an unknown flag")
	}
}

func TestDoctorVerboseHumanRuns(t *testing.T) {
	l := newSocket(t)
	t.Setenv("HERDR_PANE_ID", "w1:p2")
	done := serveRPCs(l, pongResult(), emptyList(), emptyList())
	var out bytes.Buffer
	if err := execute([]string{"doctor", "--verbose"}, nil, &out, &out); err != nil {
		t.Fatalf("doctor --verbose failed: %v\n%s", err, out.String())
	}
	waitCalls(t, l, done, 2)
	if !strings.HasPrefix(out.String(), "fledge doctor\n") {
		t.Fatalf("unexpected human header:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "configuration") {
		t.Fatalf("missing configuration block:\n%s", out.String())
	}
}

// pongResult and emptyList are wire results for the two socket calls doctor makes.
func pongResult() map[string]any {
	return map[string]any{"type": "pong", "version": "0.9.1", "protocol": 22, "capabilities": map[string]any{"live_handoff": true, "health_check": true}}
}
func emptyList() map[string]any {
	return map[string]any{"type": "integration_list", "integrations": []any{}}
}

func TestDoctorJSONSuccess(t *testing.T) {
	l := newSocket(t)
	t.Setenv("HERDR_PANE_ID", "w1:p2")
	// Serve one extra result so the listener stays open (blocked in Accept)
	// through the configuration check, which stats the still-live socket file.
	done := serveRPCs(l, pongResult(), emptyList(), emptyList())
	var out bytes.Buffer
	if err := execute([]string{"doctor", "--json"}, nil, &out, &out); err != nil {
		t.Fatalf("doctor --json failed: %v\n%s", err, out.String())
	}
	calls := waitCalls(t, l, done, 2)
	if calls[0].Method != "ping" || calls[1].Method != "integration.list" {
		t.Fatalf("unexpected calls: %+v", calls)
	}
	var doc struct {
		Checks  []struct{ Name, Status string } `json:"checks"`
		Summary struct{ OK, Warn, Fail int }    `json:"summary"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if len(doc.Checks) != 5 || doc.Summary.Fail != 0 {
		t.Fatalf("unexpected report: %+v", doc)
	}
}

func TestDoctorFailPrintsReportOnceAndExitsNonzero(t *testing.T) {
	l := newSocket(t)
	// A non-pong reply makes the connectivity check fail deterministically; the
	// empty integration list keeps model discovery from running any commands.
	done := serveRPCs(l, map[string]any{"type": "not_pong"}, emptyList())
	var out, errOut bytes.Buffer
	err := execute([]string{"doctor"}, nil, &out, &errOut)
	waitCalls(t, l, done, 2)
	if err == nil {
		t.Fatal("expected a failing doctor run to return an error")
	}
	if ExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want 1", ExitCode(err))
	}
	if !strings.Contains(out.String(), "herdr_connectivity") || !strings.Contains(out.String(), "fail") {
		t.Fatalf("report not written to stdout: %q", out.String())
	}
	// The report is printed exactly once: execute must not add an "Error:" line.
	if strings.Contains(errOut.String(), "Error:") {
		t.Fatalf("duplicate error line on stderr: %q", errOut.String())
	}
}
