package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadText(t *testing.T) {
	file := filepath.Join(t.TempDir(), "body.txt")
	if err := os.WriteFile(file, []byte("from file"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalidFile := filepath.Join(t.TempDir(), "bad.txt")
	if err := os.WriteFile(invalidFile, []byte{0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	base := TextInput{BodyFlag: "body", FileFlag: "file", Required: true, Noun: "message"}
	with := func(edit func(*TextInput)) TextInput { in := base; edit(&in); return in }
	for _, tc := range []struct {
		name  string
		input TextInput
		stdin string
		want  string
		err   string
	}{
		{"inline", with(func(in *TextInput) { in.Body, in.BodySet = "hi", true }), "", "hi", ""},
		{"file", with(func(in *TextInput) { in.File, in.FileSet = file, true }), "", "from file", ""},
		{"stdin", with(func(in *TextInput) { in.File, in.FileSet = "-", true }), "piped", "piped", ""},
		{"required missing", base, "", "", "exactly one of --body or --file is required"},
		{"required both", with(func(in *TextInput) { in.BodySet, in.FileSet = true, true }), "", "", "exactly one of --body or --file is required"},
		{"optional missing", with(func(in *TextInput) { in.Required = false }), "", "", ""},
		{"optional both", with(func(in *TextInput) { in.Required, in.BodySet, in.FileSet = false, true, true }), "", "", "at most one of --body or --file is allowed"},
		{"empty inline", with(func(in *TextInput) { in.BodySet = true }), "", "", "message must be nonempty UTF-8"},
		{"invalid UTF-8", with(func(in *TextInput) { in.File, in.FileSet = invalidFile, true }), "", "", "message must be nonempty UTF-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadText(strings.NewReader(tc.stdin), tc.input)
			var input *InputError
			if got != tc.want || tc.err == "" && err != nil || tc.err != "" && (err == nil || err.Error() != tc.err || !errors.As(err, &input)) {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
	// A missing file is a runtime failure, not invalid input.
	_, err := ReadText(nil, with(func(in *TextInput) { in.File, in.FileSet = filepath.Join(t.TempDir(), "missing"), true }))
	var input *InputError
	if err == nil || errors.As(err, &input) || !strings.HasPrefix(err.Error(), "read message: ") || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%v", err)
	}
}
