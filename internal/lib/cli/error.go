package cli

import (
	"errors"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Error is a Fledge-owned failure with a stable public code. Herdr's own
// answers stay herdr.Error.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Coded returns the public code of the first coded error in err's chain,
// Fledge's or Herdr's, and whether one was found. Neither type wraps another
// error, so a chain holds at most one of them.
func Coded(err error) (string, bool) {
	var fledge *Error
	if errors.As(err, &fledge) {
		return fledge.Code, true
	}
	var remote *herdr.Error
	if errors.As(err, &remote) {
		return remote.Code, true
	}
	return "", false
}
