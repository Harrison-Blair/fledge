package cli

import (
	"errors"
	"fmt"
	"testing"
)

type renderedError struct{}

func (renderedError) Error() string { return "rendered" }
func (renderedError) Rendered()     {}

func TestOutputError(t *testing.T) {
	cause := errors.New("output unavailable")
	err := &OutputError{Cause: cause}
	if err.Error() != cause.Error() || !errors.Is(err, cause) || err.ExitCode() != 1 {
		t.Fatalf("%v exit=%d", err, err.ExitCode())
	}
}

func TestIsRendered(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("plain"), false},
		{&OutputError{Cause: errors.New("x")}, true},
		{renderedError{}, true},
		{fmt.Errorf("wrapped: %w", renderedError{}), true},
	} {
		if got := IsRendered(tc.err); got != tc.want {
			t.Fatalf("IsRendered(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
