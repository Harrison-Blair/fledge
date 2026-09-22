// Package cli holds process-output contracts shared by command implementations.
package cli

import "errors"

// Rendered marks an error whose details were already written for the user,
// so the root command must return it without printing it again.
type Rendered interface {
	error
	Rendered()
}

// IsRendered reports whether err, or any error it wraps, is Rendered.
func IsRendered(err error) bool {
	var rendered Rendered
	return errors.As(err, &rendered)
}

// OutputError reports failure to render an outcome without attempting a second write.
type OutputError struct{ Cause error }

func (e *OutputError) Error() string { return e.Cause.Error() }
func (e *OutputError) Unwrap() error { return e.Cause }
func (e *OutputError) ExitCode() int { return 1 }
func (e *OutputError) Rendered()     {}
