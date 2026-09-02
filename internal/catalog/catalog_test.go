package catalog

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const piFake = `
test "$#" -eq 1
test "$1" = --list-models
printf '%s' "$PI_FAKE_OUTPUT"
`

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
	want := []Harness{Pi, Claude, Codex}
	if got := Harnesses(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Harnesses = %#v, want %#v", got, want)
	}
}

func TestParseHarness(t *testing.T) {
	tests := []struct {
		value   string
		want    Harness
		wantErr string
	}{
		{value: "pi", want: Pi},
		{value: "claude", want: Claude},
		{value: "codex", want: Codex},
		{value: "cursor", wantErr: `unsupported harness "cursor" (supported: pi, claude, codex)`},
		{value: "", wantErr: "harness is required (supported: pi, claude, codex)"},
		{value: "opencode", wantErr: `unsupported harness "opencode" (supported: pi, claude, codex)`},
		{value: "gemini", wantErr: `unsupported harness "gemini" (supported: pi, claude, codex)`},
	}

	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			got, err := ParseHarness(tc.value)
			if got != tc.want {
				t.Fatalf("ParseHarness(%q) = %q, want %q", tc.value, got, tc.want)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ParseHarness(%q) error = %v", tc.value, err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("ParseHarness(%q) error = %v, want %q", tc.value, err, tc.wantErr)
			}
		})
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
				"opencode/claude-fable-5",
				"opencode/claude-opus-4-8",
				"opencode-go/glm-5",
				"opencode/big-pickle",
				"openai-codex/gpt-5.5",
				"openai-codex/gpt-5.4",
				"openai-codex/gpt-5.3-codex-spark",
			},
		},
		{
			name:    "codex keeps only codex provider rows, without the provider",
			harness: Codex,
			bins:    map[string]string{"pi": piFake},
			want:    []string{"gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark"},
		},
		{
			name:    "claude keeps claude models from pi providers",
			harness: Claude,
			bins:    map[string]string{"pi": piFake},
			want:    []string{"claude-fable-5", "claude-opus-4-8"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeHarnesses(t, tc.bins)
			t.Setenv("PI_FAKE_OUTPUT", piSample)

			got := Models(context.Background(), tc.harness, time.Minute)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Models(%s) = %#v, want %#v", tc.harness, got, tc.want)
			}
		})
	}
}

func TestModelsClaudeUsesPiCatalog(t *testing.T) {
	tests := []struct {
		name string
		bins map[string]string
		want []string
	}{
		{
			name: "pi available",
			bins: map[string]string{"pi": piFake},
			want: []string{"claude-opus-4-8", "claude-pi-only"},
		},
		{
			name: "pi missing",
			bins: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeHarnesses(t, tc.bins)
			t.Setenv("PI_FAKE_OUTPUT", "provider  model  context\nopencode-go  claude-opus-4-8  1M\nopencode-go  claude-pi-only  1M\nopencode-go  big-pickle  200K\n")

			got := Models(context.Background(), Claude, time.Minute)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Models(claude) = %#v, want %#v", got, tc.want)
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
		{name: "pi exits non-zero", harness: Pi, bins: map[string]string{"pi": `exit 1`}},
		{
			name:    "pi prints a partial table then fails",
			harness: Pi,
			bins:    map[string]string{"pi": "printf '%s' \"$PI_FAKE_OUTPUT\"\nexit 1\n"},
		},
		{name: "codex source exits non-zero", harness: Codex, bins: map[string]string{"pi": `exit 1`}},
		{
			name:    "claude source exits non-zero",
			harness: Claude,
			bins:    map[string]string{"pi": `exit 1`},
		},
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
	fakeHarnesses(t, map[string]string{"pi": "exec " + sleepBin + " 2"})

	start := time.Now()
	got := Models(context.Background(), Pi, 100*time.Millisecond)
	elapsed := time.Since(start)

	if len(got) != 0 {
		t.Fatalf("Models(pi) = %#v, want none", got)
	}
	// The fake sleeps for 2s, so any bound below that proves run stopped
	// waiting on the killed child rather than outliving it.
	if elapsed > 1500*time.Millisecond {
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

func TestModelsNeverInvokesOpenCode(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "invoked")
	fakeHarnesses(t, map[string]string{"opencode": `: > "$OPENCODE_MARKER"`})
	t.Setenv("OPENCODE_MARKER", marker)

	for _, harness := range Harnesses() {
		Models(context.Background(), harness, time.Minute)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("opencode executable invocation marker error = %v, want not exist", err)
	}
}

func TestModelsRejectsNonPositiveTimeout(t *testing.T) {
	fakeHarnesses(t, map[string]string{"pi": piFake})
	t.Setenv("PI_FAKE_OUTPUT", piSample)

	if got := Models(context.Background(), Pi, 0); len(got) != 0 {
		t.Fatalf("Models(pi) = %#v, want none", got)
	}
}
