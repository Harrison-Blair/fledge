package catalog

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
)

// cursorSample stands in for authenticated cursor-agent --list-models output.
// That format is unverified, so the fake pads a column and emits an error line
// to exercise the leading-token and skip rules.
const cursorSample = `gpt-5.5  (default)
claude-opus-4-8
Error: partial listing failure
`

const (
	piFake = `
test "$#" -eq 1
test "$1" = --list-models
printf '%s' "$PI_FAKE_OUTPUT"
`
	openCodeFake = `
test "$#" -eq 1
test "$1" = models
printf '%s' "$OPENCODE_FAKE_OUTPUT"
`
	cursorFake = `
test "$#" -eq 1
test "$1" = --list-models
printf '%s' "$CURSOR_FAKE_OUTPUT"
`
)

// fakeHarnesses replaces PATH with a directory holding only these stand-ins,
// so a harness the case leaves out is genuinely missing instead of resolving
// to a real binary installed on the machine.
func fakeHarnesses(t *testing.T, bins map[string]string) {
	t.Helper()
	binDir := t.TempDir()
	for name, script := range bins {
		contents := "#!/bin/sh\nset -eu\n" + script
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(contents), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)
}

func TestHarnesses(t *testing.T) {
	want := []Harness{Pi, Claude, Codex, OpenCode, Cursor}
	if got := Harnesses(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Harnesses = %#v, want %#v", got, want)
	}
}

func TestModels(t *testing.T) {
	tests := []struct {
		name    string
		harness Harness
		bins    map[string]string
		want    []string
	}{
		{
			name:    "pi qualifies models by provider",
			harness: Pi,
			bins:    map[string]string{"pi": piFake},
			want: []string{
				"openai-codex/gpt-5.3-codex-spark",
				"openai-codex/gpt-5.4",
				"openai-codex/gpt-5.5",
				"opencode/big-pickle",
				"opencode/claude-fable-5",
				"opencode/claude-opus-4-8",
			},
		},
		{
			name:    "opencode reports whole lines",
			harness: OpenCode,
			bins:    map[string]string{"opencode": openCodeFake},
			want: []string{
				"ollama/llama3",
				"opencode/big-pickle",
				"opencode/claude-fable-5",
				"opencode/claude-opus-4-8",
				"opencode/deepseek-v4-flash",
			},
		},
		{
			name:    "codex keeps only codex provider rows, without the provider",
			harness: Codex,
			bins:    map[string]string{"pi": piFake},
			want:    []string{"gpt-5.3-codex-spark", "gpt-5.4", "gpt-5.5"},
		},
		{
			name:    "claude merges both catalogs",
			harness: Claude,
			bins:    map[string]string{"pi": piFake, "opencode": openCodeFake},
			want:    []string{"claude-fable-5", "claude-opus-4-8"},
		},
		{
			name:    "cursor takes the leading token and skips error lines",
			harness: Cursor,
			bins:    map[string]string{"cursor-agent": cursorFake},
			want:    []string{"claude-opus-4-8", "gpt-5.5"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeHarnesses(t, tc.bins)
			t.Setenv("PI_FAKE_OUTPUT", piSample)
			t.Setenv("OPENCODE_FAKE_OUTPUT", openCodeSample)
			t.Setenv("CURSOR_FAKE_OUTPUT", cursorSample)

			got := Models(context.Background(), tc.harness, time.Minute)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Models(%s) = %#v, want %#v", tc.harness, got, tc.want)
			}
		})
	}
}

func TestModelsClaudeUnionsAvailableSources(t *testing.T) {
	tests := []struct {
		name string
		bins map[string]string
		want []string
	}{
		{
			name: "both sources",
			bins: map[string]string{"pi": piFake, "opencode": openCodeFake},
			want: []string{"claude-opus-4-8", "claude-pi-only", "claude-opencode-only"},
		},
		{
			name: "opencode missing",
			bins: map[string]string{"pi": piFake},
			want: []string{"claude-opus-4-8", "claude-pi-only"},
		},
		{
			name: "pi missing",
			bins: map[string]string{"opencode": openCodeFake},
			want: []string{"claude-opus-4-8", "claude-opencode-only"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeHarnesses(t, tc.bins)
			t.Setenv("PI_FAKE_OUTPUT", "provider  model  context\nopencode  claude-opus-4-8  1M\nopencode  claude-pi-only  1M\nopencode  big-pickle  200K\n")
			t.Setenv("OPENCODE_FAKE_OUTPUT", "opencode/claude-opus-4-8\nopencode/claude-opencode-only\nollama/llama3\n")

			want := slices.Sorted(slices.Values(tc.want))
			got := Models(context.Background(), Claude, time.Minute)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Models(claude) = %#v, want %#v", got, want)
			}
		})
	}
}

func TestModelsIgnoresCommandFailure(t *testing.T) {
	tests := []struct {
		name    string
		harness Harness
		bins    map[string]string
	}{
		{
			name:    "cursor is unauthenticated",
			harness: Cursor,
			bins: map[string]string{"cursor-agent": `
printf "Error: Authentication required. Run 'agent login', pass --api-key/--auth-token, or set CURSOR_API_KEY/CURSOR_AUTH_TOKEN.\n" >&2
exit 1
`},
		},
		{name: "pi exits non-zero", harness: Pi, bins: map[string]string{"pi": `exit 1`}},
		{
			name:    "pi prints a partial table then fails",
			harness: Pi,
			bins:    map[string]string{"pi": "printf '%s' \"$PI_FAKE_OUTPUT\"\nexit 1\n"},
		},
		{name: "codex source exits non-zero", harness: Codex, bins: map[string]string{"pi": `exit 1`}},
		{
			name:    "both claude sources exit non-zero",
			harness: Claude,
			bins:    map[string]string{"pi": `exit 1`, "opencode": `exit 1`},
		},
		{name: "opencode exits non-zero", harness: OpenCode, bins: map[string]string{"opencode": `exit 1`}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeHarnesses(t, tc.bins)
			t.Setenv("PI_FAKE_OUTPUT", piSample)
			if got := Models(context.Background(), tc.harness, time.Minute); len(got) != 0 {
				t.Fatalf("Models(%s) = %#v, want none", tc.harness, got)
			}
		})
	}
}

func TestModelsIgnoresTimeout(t *testing.T) {
	// PATH holds only the fakes, so the sleep the fake blocks on needs its
	// resolved location.
	sleepBin, err := exec.LookPath("sleep")
	if err != nil {
		t.Fatal(err)
	}
	fakeHarnesses(t, map[string]string{"pi": sleepBin + " 2"})

	start := time.Now()
	got := Models(context.Background(), Pi, 100*time.Millisecond)
	elapsed := time.Since(start)

	if len(got) != 0 {
		t.Fatalf("Models(pi) = %#v, want none", got)
	}
	if elapsed > time.Second {
		t.Fatalf("Models(pi) took %v, want a prompt return", elapsed)
	}
}

func TestModelsIgnoresMissingBinary(t *testing.T) {
	for _, harness := range Harnesses() {
		t.Run(string(harness), func(t *testing.T) {
			t.Setenv("PATH", t.TempDir())
			if got := Models(context.Background(), harness, time.Minute); len(got) != 0 {
				t.Fatalf("Models(%s) = %#v, want none", harness, got)
			}
		})
	}
}

func TestModelsRejectsUnknownHarness(t *testing.T) {
	fakeHarnesses(t, map[string]string{"pi": piFake})
	t.Setenv("PI_FAKE_OUTPUT", piSample)

	if got := Models(context.Background(), Harness("gemini"), time.Minute); got != nil {
		t.Fatalf("Models(gemini) = %#v, want nil", got)
	}
}
