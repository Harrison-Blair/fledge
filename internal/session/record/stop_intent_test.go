package record

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStopIntentRoundTripAndReplacement(t *testing.T) {
	record := Record{HerdrSessionName: "managed", Path: t.TempDir()}
	first := strings.Repeat("1", 32)
	second := strings.Repeat("a", 32)
	if err := WriteStopIntent(record, first); err != nil {
		t.Fatalf("WriteStopIntent(first): %v", err)
	}
	if err := WriteStopIntent(record, second); err != nil {
		t.Fatalf("WriteStopIntent(second): %v", err)
	}
	got, err := ReadStopIntent(record)
	if err != nil {
		t.Fatalf("ReadStopIntent: %v", err)
	}
	if !got.Exists || got.ID != second {
		t.Fatalf("ReadStopIntent = %#v, want %q", got, second)
	}
	info, err := os.Lstat(filepath.Join(record.Path, StopIntentFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o644 {
		t.Fatalf("stop intent mode = %v", info.Mode())
	}
	matches, err := filepath.Glob(filepath.Join(record.Path, ".stop-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary stop files = %q, error = %v", matches, err)
	}
}

func TestStopIntentRejectsMalformedState(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "unknown field", data: `{"schema_version":1,"intent_id":"00000000000000000000000000000000","extra":true}`, want: "unknown field"},
		{name: "wrong version", data: `{"schema_version":2,"intent_id":"00000000000000000000000000000000"}`, want: "unsupported schema_version"},
		{name: "uppercase", data: `{"schema_version":1,"intent_id":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`, want: "lowercase hexadecimal"},
		{name: "short", data: `{"schema_version":1,"intent_id":"00"}`, want: "32 lowercase"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := Record{HerdrSessionName: "managed", Path: t.TempDir()}
			if err := os.WriteFile(filepath.Join(record.Path, StopIntentFileName), []byte(test.data), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := ReadStopIntent(record)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ReadStopIntent() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestGenerateStopIntentUsesExactly128Bits(t *testing.T) {
	id, err := GenerateStopIntent(bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}))
	if err != nil {
		t.Fatal(err)
	}
	if id != "000102030405060708090a0b0c0d0e0f" {
		t.Fatalf("intent ID = %q", id)
	}
}
