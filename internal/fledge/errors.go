package fledge

import (
	"errors"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

type Error struct {
	Code    string
	Message string
	Details any
	Cause   error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

func NewError(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func Wrap(code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// isMessagingFailure reports whether err is a durable-messaging failure.
// Callers that would otherwise reclassify a run-close failure as a state
// problem pass these through unchanged.
func isMessagingFailure(err error) bool {
	var serviceErr *Error
	return errors.As(err, &serviceErr) && strings.HasPrefix(serviceErr.Code, "message_")
}

func Translate(err error) *Error {
	if err == nil {
		return nil
	}
	var fledgeErr *Error
	if errors.As(err, &fledgeErr) {
		return fledgeErr
	}
	var apiErr *herdr.APIError
	if errors.As(err, &apiErr) {
		return &Error{
			Code:    "herdr_" + apiErr.Code,
			Message: apiErr.Message,
			Details: map[string]string{"herdr_code": apiErr.Code},
			Cause:   err,
		}
	}
	var transportErr *herdr.TransportError
	if errors.As(err, &transportErr) {
		return Wrap("herdr_transport", transportErr.Error(), err)
	}
	return Wrap("runtime_error", err.Error(), err)
}
