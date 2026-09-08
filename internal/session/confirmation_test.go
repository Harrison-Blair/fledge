package session

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"fledge/internal/session/sessiontest"
)

func TestTerminalConfirmerAcceptsOnlyYOrYes(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		answer bool
	}{
		{name: "lowercase y", input: "y\n", answer: true},
		{name: "uppercase yes", input: "YES\n", answer: true},
		{name: "surrounding whitespace", input: "  Yes  \n", answer: false},
		{name: "blank", input: "\n", answer: false},
		{name: "negative", input: "no\n", answer: false},
		{name: "invalid", input: "sure\n", answer: false},
		{name: "EOF", input: "", answer: false},
		{name: "affirmative then EOF", input: "yes", answer: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			got, err := (TerminalConfirmer{
				Input:            strings.NewReader(test.input),
				Output:           &output,
				InputIsTerminal:  true,
				OutputIsTerminal: true,
			}).Confirm("/project", []string{"running"}, false)
			if err != nil {
				t.Fatalf("Confirm() error = %v", err)
			}
			if got != test.answer {
				t.Fatalf("Confirm() = %v, want %v", got, test.answer)
			}
		})
	}
}

func TestTerminalConfirmerRequiresBothTerminals(t *testing.T) {
	tests := []struct {
		name      string
		inputTTY  bool
		outputTTY bool
	}{
		{name: "neither", inputTTY: false, outputTTY: false},
		{name: "input only", inputTTY: true, outputTTY: false},
		{name: "output only", inputTTY: false, outputTTY: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			_, err := (TerminalConfirmer{
				Input:            strings.NewReader("yes\n"),
				Output:           &output,
				InputIsTerminal:  test.inputTTY,
				OutputIsTerminal: test.outputTTY,
			}).Confirm("/project", []string{"running"}, false)
			if err == nil || !strings.Contains(err.Error(), "terminal") {
				t.Fatalf("Confirm() error = %v, want terminal error", err)
			}
			if output.Len() != 0 {
				t.Fatalf("Confirm() wrote prompt on non-terminal: %q", output.String())
			}
		})
	}
}

func TestTerminalConfirmerPromptIsDeterministicAndWarnsForSelfStop(t *testing.T) {
	names := []string{"zeta", "alpha"}
	var output bytes.Buffer
	confirmed, err := (TerminalConfirmer{
		Input:            strings.NewReader("no\n"),
		Output:           &output,
		InputIsTerminal:  true,
		OutputIsTerminal: true,
	}).Confirm("/canonical/project", names, true)
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if confirmed {
		t.Fatal("Confirm() = true, want cancellation")
	}
	want := "Stop Fledge sessions for project \"/canonical/project\"?\n" +
		"  - alpha\n" +
		"  - zeta\n" +
		"This will terminate all panes and agents in these sessions.\n" +
		"The current Herder session is included; Fledge may exit before showing final output.\n" +
		"Continue? [y/N] "
	if output.String() != want {
		t.Fatalf("prompt = %q, want %q", output.String(), want)
	}
	if !reflect.DeepEqual(names, []string{"zeta", "alpha"}) {
		t.Fatalf("Confirm() mutated names: %#v", names)
	}
}

func TestTerminalConfirmerReturnsReadAndWriteFailures(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		want := errors.New("interrupt")
		_, err := (TerminalConfirmer{
			Input:            sessiontest.ErrorReader{Err: want},
			Output:           io.Discard,
			InputIsTerminal:  true,
			OutputIsTerminal: true,
		}).Confirm("/project", []string{"running"}, false)
		if !errors.Is(err, want) {
			t.Fatalf("Confirm() error = %v, want wrapped %v", err, want)
		}
	})

	t.Run("write", func(t *testing.T) {
		want := errors.New("write failed")
		_, err := (TerminalConfirmer{
			Input:            strings.NewReader("yes\n"),
			Output:           failingWriter{err: want},
			InputIsTerminal:  true,
			OutputIsTerminal: true,
		}).Confirm("/project", []string{"running"}, false)
		if !errors.Is(err, want) {
			t.Fatalf("Confirm() error = %v, want wrapped %v", err, want)
		}
	})
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}
