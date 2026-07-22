package main

import (
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/protocol"
)

// takeFlag must not swallow a flag-shaped token into a preceding flag's value
// slot. This CLI has no flags whose value is a negative number, so any value
// beginning with "-" is treated as a missing value.
func TestTakeFlag(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		long       string
		short      string
		wantValue  string
		wantRest   []string
		wantErrSub string
	}{
		{
			name: "value follows long flag",
			args: []string{"--model", "gpt5", "extra"},
			long: "--model", short: "-M",
			wantValue: "gpt5",
			wantRest:  []string{"extra"},
		},
		{
			name: "value follows short flag",
			args: []string{"-M", "gpt5"},
			long: "--model", short: "-M",
			wantValue: "gpt5",
			wantRest:  nil,
		},
		{
			name: "flag absent",
			args: []string{"extra"},
			long: "--model", short: "-M",
			wantRest: []string{"extra"},
		},
		{
			name: "flag at end has no value",
			args: []string{"--model"},
			long: "--model", short: "-M",
			wantErrSub: "missing value",
		},
		{
			// Repro: --model consumed --profile as its value; the later
			// rejectFlags sweep then never saw the stray flag.
			name: "flag-shaped value is rejected",
			args: []string{"--model", "--profile", "foo"},
			long: "--model", short: "-M",
			wantErrSub: "missing value",
		},
		{
			name: "short-flag-shaped value is rejected",
			args: []string{"--species", "-P", "1234"},
			long: "--species", short: "-S",
			wantErrSub: "missing value",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value, rest, err := takeFlag(tc.args, tc.long, tc.short)
			if tc.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if value != tc.wantValue {
				t.Errorf("value = %q, want %q", value, tc.wantValue)
			}
			if strings.Join(rest, "\x00") != strings.Join(tc.wantRest, "\x00") {
				t.Errorf("rest = %q, want %q", rest, tc.wantRest)
			}
		})
	}
}

func TestTakeBoolFlag(t *testing.T) {
	found, rest := takeBoolFlag([]string{"a", "--json", "b"}, "--json", "-J")
	if !found || strings.Join(rest, ",") != "a,b" {
		t.Fatalf("found=%v rest=%v", found, rest)
	}
	found, rest = takeBoolFlag([]string{"a", "-J"}, "--json", "-J")
	if !found || strings.Join(rest, ",") != "a" {
		t.Fatalf("short: found=%v rest=%v", found, rest)
	}
	found, rest = takeBoolFlag([]string{"a", "b"}, "--json", "-J")
	if found || strings.Join(rest, ",") != "a,b" {
		t.Fatalf("absent: found=%v rest=%v", found, rest)
	}
}

func TestRejectFlags(t *testing.T) {
	if err := rejectFlags("cmd", []string{"a", "b"}); err != nil {
		t.Fatalf("clean args errored: %v", err)
	}
	err := rejectFlags("cmd", []string{"a", "--stray"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("err = %v, want unknown flag", err)
	}
}

// End-to-end repro: a flag-shaped value used to be swallowed, so this
// registered agent type "1234" with species "--pid" instead of erroring.
func TestAgentRegisterRejectsFlagShapedSpecies(t *testing.T) {
	commandWorkspace(t)
	_, err := captureRun(t, "agent", "register", "--species", "--pid", "1234")
	// Before the fix, --species swallowed "--pid" and the command reached the
	// daemon (erroring only on downstream species validation). The parser must
	// instead reject the missing value up front.
	if err == nil || !strings.Contains(err.Error(), "--species: missing value") {
		t.Fatalf("register err = %v, want --species: missing value", err)
	}
}

// agent msg wait --timeout must reject a non-positive duration; the daemon
// gates on TimeoutMS > 0, so a zero or negative timeout would otherwise wait
// forever.
func TestAgentMsgWaitRejectsNonPositiveTimeout(t *testing.T) {
	commandWorkspace(t)
	name := registerCLI(t, "worker")
	t.Setenv(protocol.AgentNameEnv, name)
	for _, dur := range []string{"0s", "-5s"} {
		if _, err := captureRun(t, "agent", "msg", "wait", "--timeout", dur); err == nil {
			t.Errorf("wait --timeout %s succeeded, want error", dur)
		}
	}
}

// agent msg send must not treat a flag-shaped recipient/body as a help
// request and silently drop the message. Only a lone leading help flag is help.
func TestAgentMsgSendDoesNotHelpOnFlagShapedPositionals(t *testing.T) {
	commandWorkspace(t)
	name := registerCLI(t, "worker")
	t.Setenv(protocol.AgentNameEnv, name)
	for _, args := range [][]string{
		{"agent", "msg", "send", name, "-H"},
		{"agent", "msg", "send", name, "--help"},
		{"agent", "msg", "send", "--help", "body"},
	} {
		out, err := captureRun(t, args...)
		if err == nil {
			t.Errorf("run(%q) succeeded (likely printed help and dropped message): %q", args, out)
		}
		if strings.Contains(out, "usage:") {
			t.Errorf("run(%q) printed help text: %q", args, out)
		}
	}
	// The legitimate lone help form is preserved.
	out, err := captureRun(t, "agent", "msg", "send", "--help")
	if err != nil || !strings.Contains(out, "usage:") {
		t.Errorf("send --help: out=%q err=%v, want help text", out, err)
	}
}

// fledge daemon <bogus> must name the invalid token, not the parent command.
func TestDaemonUnknownSubcommandNamesToken(t *testing.T) {
	err := runDaemon([]string{"bogus"})
	if err == nil || !strings.Contains(err.Error(), `"bogus"`) {
		t.Fatalf("err = %v, want naming \"bogus\"", err)
	}
}
