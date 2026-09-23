package agent

import (
	"fmt"
	"io"
	"os"
	"unicode/utf8"
)

// TextInput configures ReadText's inline/file resolution and its error wording.
// Required demands exactly one of BodyFlag/FileFlag; otherwise at most one is
// allowed, and neither set returns an empty, error-free result. Noun names the
// text being read (e.g. "message" or "prompt") in error messages.
type TextInput struct {
	Body, BodyFlag string
	BodySet        bool
	File, FileFlag string
	FileSet        bool
	Required       bool
	Noun           string
}

// ReadText resolves inline, file, or stdin ("-") text as nonempty UTF-8.
func ReadText(in io.Reader, t TextInput) (string, error) {
	if t.Required {
		if t.BodySet == t.FileSet {
			return "", Invalid("exactly one of --%s or --%s is required", t.BodyFlag, t.FileFlag)
		}
	} else if t.BodySet && t.FileSet {
		return "", Invalid("at most one of --%s or --%s is allowed", t.BodyFlag, t.FileFlag)
	}
	if !t.BodySet && !t.FileSet {
		return "", nil
	}
	text := t.Body
	if t.FileSet {
		var b []byte
		var err error
		if t.File == "-" {
			b, err = io.ReadAll(in)
		} else {
			b, err = os.ReadFile(t.File)
		}
		if err != nil {
			return "", fmt.Errorf("read %s: %w", t.Noun, err)
		}
		text = string(b)
	}
	if text == "" || !utf8.ValidString(text) {
		return "", Invalid("%s must be nonempty UTF-8", t.Noun)
	}
	return text, nil
}
