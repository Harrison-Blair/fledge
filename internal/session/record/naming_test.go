package record

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"fledge/internal/session/sessiontest"
)

// nameStamp is a fixed UTC instant; nameStampText is its rendered timestamp as
// it appears in a generated session name.
var nameStamp = time.Date(2026, 9, 2, 14, 35, 12, 0, time.UTC)

const nameStampText = "2026-09-02T14.35.12Z"

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

func TestGenerateNamePlacesUTCTimestampBeforeSlug(t *testing.T) {
	got, err := GenerateName("My Project", nameStamp, MaxSessionLength, nil, bytes.NewReader([]byte{0xab, 0xcd, 0xef, 0x12}))
	if err != nil {
		t.Fatalf("GenerateName() error = %v", err)
	}
	want := "fledge-2026-09-02T14.35.12Z-My-Project-abcdef12"
	if got != want {
		t.Fatalf("GenerateName() = %q, want %q", got, want)
	}
}

func TestGenerateNameRetriesUnavailableNames(t *testing.T) {
	unavailable := map[string]struct{}{
		"fledge-" + nameStampText + "-Project-00000001": {},
	}
	entropy := bytes.NewReader([]byte{
		0, 0, 0, 1,
		0xab, 0xcd, 0xef, 0x12,
	})

	got, err := GenerateName("Project", nameStamp, MaxSessionLength, unavailable, entropy)
	if err != nil {
		t.Fatalf("GenerateName() error = %v", err)
	}
	// The timestamp is stable across the retry; only the random suffix advances.
	want := "fledge-" + nameStampText + "-Project-abcdef12"
	if got != want {
		t.Fatalf("GenerateName() = %q, want %q", got, want)
	}
}

func TestGenerateNameLimitsNameTo64Bytes(t *testing.T) {
	got, err := GenerateName(strings.Repeat("a", 80), nameStamp, MaxSessionLength, nil, bytes.NewReader([]byte{1, 2, 3, 4}))
	if err != nil {
		t.Fatalf("GenerateName() error = %v", err)
	}
	// The normal 64-byte limit caps the slug at 27 bytes.
	want := "fledge-" + nameStampText + "-" + strings.Repeat("a", 27) + "-01020304"
	if got != want {
		t.Fatalf("GenerateName() = %q, want %q", got, want)
	}
	if len(got) != MaxSessionLength {
		t.Fatalf("len(GenerateName()) = %d, want %d: %q", len(got), MaxSessionLength, got)
	}
}

func TestGenerateNameShrinksSlugToLowerCapacity(t *testing.T) {
	const limit = 50
	got, err := GenerateName(strings.Repeat("a", 80), nameStamp, limit, nil, bytes.NewReader([]byte{1, 2, 3, 4}))
	if err != nil {
		t.Fatalf("GenerateName() error = %v", err)
	}
	want := "fledge-" + nameStampText + "-" + strings.Repeat("a", limit-fixedNameCost) + "-01020304"
	if got != want {
		t.Fatalf("GenerateName() = %q, want %q", got, want)
	}
	if len(got) != limit {
		t.Fatalf("len(GenerateName()) = %d, want %d: %q", len(got), limit, got)
	}
}

func TestGenerateNameReportsEntropyFailure(t *testing.T) {
	want := errors.New("entropy failed")
	_, err := GenerateName("project", nameStamp, MaxSessionLength, nil, sessiontest.ErrorReader{Err: want})
	if !errors.Is(err, want) {
		t.Fatalf("GenerateName() error = %v, want wrapped %v", err, want)
	}
}

func TestGenerateNameRejectsTooShortLimitBeforeReadingEntropy(t *testing.T) {
	entropy := &sessiontest.CountingReader{}
	// One byte below the minimum leaves no room for even a single-byte slug.
	_, err := GenerateName("project", nameStamp, MinSessionLength-1, nil, entropy)
	if err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("GenerateName() error = %v, want too-short error", err)
	}
	if entropy.Reads != 0 {
		t.Fatalf("GenerateName() entropy reads = %d, want 0", entropy.Reads)
	}
}

func TestGenerateNameAcceptsMinimumAndClampsLargeLimit(t *testing.T) {
	got, err := GenerateName("project", nameStamp, MinSessionLength, nil, bytes.NewReader([]byte{1, 2, 3, 4}))
	if err != nil {
		t.Fatalf("GenerateName() at minimum error = %v", err)
	}
	// At the 38-byte minimum the slug is truncated to one byte.
	want := "fledge-" + nameStampText + "-p-01020304"
	if got != want {
		t.Fatalf("GenerateName() at minimum = %q, want %q", got, want)
	}
	if len(got) != MinSessionLength {
		t.Fatalf("len(GenerateName()) at minimum = %d, want %d", len(got), MinSessionLength)
	}

	got, err = GenerateName(strings.Repeat("a", 80), nameStamp, MaxSessionLength+1, nil, bytes.NewReader([]byte{0xab, 0xcd, 0xef, 0x12}))
	if err != nil {
		t.Fatalf("GenerateName() above maximum error = %v", err)
	}
	if len(got) != MaxSessionLength || !strings.HasSuffix(got, "-abcdef12") {
		t.Fatalf("GenerateName() above maximum = %q, want 64-byte clamped name with full hash", got)
	}
}
