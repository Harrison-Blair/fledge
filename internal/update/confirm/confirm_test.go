package confirm_test

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/update/confirm"
)

func terminal() bool { return true }

func TestProceedAccepts(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "  y  \n", "y"} {
		var out bytes.Buffer
		ok, err := confirm.Proceed(strings.NewReader(answer), &out, terminal, "v0.2.0", "v0.1.0")
		if err != nil || !ok {
			t.Fatalf("answer %q: Proceed() = %v, %v", answer, ok, err)
		}
		if out.String() != "Install fledge v0.2.0 over v0.1.0? [y/N] " {
			t.Fatalf("answer %q: output = %q", answer, out.String())
		}
	}
}

func TestProceedCancels(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", "", "yes\n"} {
		var out bytes.Buffer
		ok, err := confirm.Proceed(strings.NewReader(answer), &out, terminal, "v0.2.0", "v0.1.0")
		if err != nil || ok {
			t.Fatalf("answer %q: Proceed() = %v, %v", answer, ok, err)
		}
		if out.String() != "Install fledge v0.2.0 over v0.1.0? [y/N] update canceled\n" {
			t.Fatalf("answer %q: output = %q", answer, out.String())
		}
	}
}

func TestProceedRequiresTerminal(t *testing.T) {
	var out bytes.Buffer
	ok, err := confirm.Proceed(strings.NewReader("y\n"), &out, func() bool { return false }, "v0.2.0", "v0.1.0")
	if ok || err == nil || err.Error() != "update: stdin is not a terminal; use --yes to update without a prompt" {
		t.Fatalf("Proceed() = %v, %v", ok, err)
	}
	if out.Len() != 0 {
		t.Fatalf("prompted without a terminal: %q", out.String())
	}
}

func TestNullDeviceIsNotAnInteractiveTerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if ok, err := confirm.Proceed(f, new(bytes.Buffer), nil, "v0.2.0", "v0.1.0"); ok || err == nil || !strings.Contains(err.Error(), "not a terminal") {
		t.Fatalf("expected --yes requirement for /dev/null, got %v, %v", ok, err)
	}
}

func TestNonFileInputIsNotATerminal(t *testing.T) {
	if ok, err := confirm.Proceed(strings.NewReader("y\n"), new(bytes.Buffer), nil, "v0.2.0", "v0.1.0"); ok || err == nil || !strings.Contains(err.Error(), "not a terminal") {
		t.Fatalf("got %v, %v", ok, err)
	}
}

type failingIO struct{ err error }

func (f failingIO) Read([]byte) (int, error)  { return 0, f.err }
func (f failingIO) Write([]byte) (int, error) { return 0, f.err }

type writeFailureAfter struct {
	remaining int
	err       error
}

func (w *writeFailureAfter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

func TestProceedIOFailures(t *testing.T) {
	sentinel := errors.New("I/O failed")
	if _, err := confirm.Proceed(failingIO{sentinel}, new(bytes.Buffer), terminal, "v0.2.0", "v0.1.0"); !errors.Is(err, sentinel) || !strings.HasPrefix(err.Error(), "update: read confirmation: ") {
		t.Fatalf("read failure: %v", err)
	}
	if _, err := confirm.Proceed(strings.NewReader("y\n"), failingIO{sentinel}, terminal, "v0.2.0", "v0.1.0"); !errors.Is(err, sentinel) {
		t.Fatalf("prompt failure: %v", err)
	}
	if ok, err := confirm.Proceed(strings.NewReader("n\n"), &writeFailureAfter{remaining: 1, err: sentinel}, terminal, "v0.2.0", "v0.1.0"); ok || !errors.Is(err, sentinel) {
		t.Fatalf("cancel output failure: %v, %v", ok, err)
	}
}
