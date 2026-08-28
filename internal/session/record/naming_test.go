package record

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"fledge/internal/session/sessiontest"
)

func TestSlug(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "preserves valid characters and case", in: "My.Project_name-2", want: "My.Project_name-2"},
		{name: "replaces invalid runs", in: "my  weird/项目", want: "my-weird"},
		{name: "trims edge separators", in: "._-project-_.", want: "project"},
		{name: "falls back", in: "项目 😀", want: "project"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Slug(test.in); got != test.want {
				t.Fatalf("Slug(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestGenerateNameRetriesUnavailableNames(t *testing.T) {
	unavailable := map[string]struct{}{
		"fledge-Project-00000001": {},
	}
	entropy := bytes.NewReader([]byte{
		0, 0, 0, 1,
		0xab, 0xcd, 0xef, 0x12,
	})

	got, err := GenerateName("Project", MaxSessionLength, unavailable, entropy)
	if err != nil {
		t.Fatalf("GenerateName() error = %v", err)
	}
	if got != "fledge-Project-abcdef12" {
		t.Fatalf("GenerateName() = %q, want %q", got, "fledge-Project-abcdef12")
	}
}

func TestGenerateNameLimitsNameTo64Bytes(t *testing.T) {
	got, err := GenerateName(strings.Repeat("a", 80), MaxSessionLength, nil, bytes.NewReader([]byte{1, 2, 3, 4}))
	if err != nil {
		t.Fatalf("GenerateName() error = %v", err)
	}
	if len(got) != 64 {
		t.Fatalf("len(GenerateName()) = %d, want 64: %q", len(got), got)
	}
	if !strings.HasSuffix(got, "-01020304") {
		t.Fatalf("GenerateName() = %q, want random suffix", got)
	}
}

func TestGenerateNameReportsEntropyFailure(t *testing.T) {
	want := errors.New("entropy failed")
	_, err := GenerateName("project", MaxSessionLength, nil, sessiontest.ErrorReader{Err: want})
	if !errors.Is(err, want) {
		t.Fatalf("GenerateName() error = %v, want wrapped %v", err, want)
	}
}

func TestGenerateNameRejectsTooShortLimitBeforeReadingEntropy(t *testing.T) {
	entropy := &sessiontest.CountingReader{}
	_, err := GenerateName("project", MinSessionLength-1, nil, entropy)
	if err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("GenerateName() error = %v, want too-short error", err)
	}
	if entropy.Reads != 0 {
		t.Fatalf("GenerateName() entropy reads = %d, want 0", entropy.Reads)
	}
}

func TestGenerateNameAcceptsMinimumAndClampsLargeLimit(t *testing.T) {
	got, err := GenerateName("project", MinSessionLength, nil, bytes.NewReader([]byte{1, 2, 3, 4}))
	if err != nil {
		t.Fatalf("GenerateName() at minimum error = %v", err)
	}
	if got != "fledge-p-01020304" {
		t.Fatalf("GenerateName() at minimum = %q, want shortest valid name", got)
	}

	got, err = GenerateName(strings.Repeat("a", 80), MaxSessionLength+1, nil, bytes.NewReader([]byte{0xab, 0xcd, 0xef, 0x12}))
	if err != nil {
		t.Fatalf("GenerateName() above maximum error = %v", err)
	}
	if len(got) != MaxSessionLength || !strings.HasSuffix(got, "-abcdef12") {
		t.Fatalf("GenerateName() above maximum = %q, want 64-byte clamped name with full hash", got)
	}
}
