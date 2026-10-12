package cli

// AtPhase attributes err to an operation phase, which Outcome.Fail reports in
// place of the caller's default phase.
func AtPhase(phase string, err error) error { return &phaseError{phase: phase, cause: err} }

type phaseError struct {
	phase string
	cause error
}

func (e *phaseError) Error() string { return e.cause.Error() }
func (e *phaseError) Unwrap() error { return e.cause }
