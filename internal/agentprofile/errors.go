package agentprofile

import (
	"errors"
	"fmt"
)

// Kind classifies an operation failure without requiring callers to inspect
// error text.
type Kind uint8

const (
	KindInvalid Kind = iota + 1
	KindAlreadyExists
	KindNotFound
)

var (
	// ErrInvalid reports an invalid profile value, document, path, or file type.
	ErrInvalid = errors.New("invalid agent profile")
	// ErrAlreadyExists reports that Create would replace an existing profile.
	ErrAlreadyExists = errors.New("agent profile already exists")
	// ErrNotFound reports that the requested profile does not exist.
	ErrNotFound = errors.New("agent profile not found")
)

// ValidationError identifies the profile field that violates an invariant.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Reason
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalid
}

// Error adds operation context and a stable classification to store errors.
type Error struct {
	Kind Kind
	Op   string
	Name string
	Path string
	Err  error
}

func (e *Error) Error() string {
	object := "agent profile"
	if e.Name != "" {
		object += fmt.Sprintf(" %q", e.Name)
	}
	if e.Op != "" {
		object = e.Op + " " + object
	}
	if e.Err != nil {
		return object + ": " + e.Err.Error()
	}
	return object + ": " + e.sentinel().Error()
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Is(target error) bool {
	return target == e.sentinel() || errors.Is(e.Err, target)
}

func (e *Error) sentinel() error {
	switch e.Kind {
	case KindAlreadyExists:
		return ErrAlreadyExists
	case KindNotFound:
		return ErrNotFound
	default:
		return ErrInvalid
	}
}

func invalid(op, name, path string, err error) error {
	return &Error{Kind: KindInvalid, Op: op, Name: name, Path: path, Err: err}
}

func alreadyExists(op, name, path string, err error) error {
	return &Error{Kind: KindAlreadyExists, Op: op, Name: name, Path: path, Err: err}
}

func notFound(op, name, path string, err error) error {
	return &Error{Kind: KindNotFound, Op: op, Name: name, Path: path, Err: err}
}
