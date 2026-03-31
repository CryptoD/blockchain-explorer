package apperrors

import (
	"errors"
	"fmt"
)

// ErrNotFound is the sentinel for missing resources. Use with [errors.Is]; maps to API code [CodeNotFound].
var ErrNotFound = errors.New("not found")

// Stable API error code strings (JSON error.code).
const (
	CodeNotFound    = "not_found"
	CodeInternal    = "internal_error"
	CodeBadRequest  = "bad_request"
	CodeUnavailable = "service_unavailable"
)

// Error is a domain error with a stable client-facing code and HTTP status.
// The optional Err field is for logging and [errors.Is]/[errors.As]; it is not sent to clients by default.
type Error struct {
	Code    string
	Status  int
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the wrapped error for errors.Is / errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// New returns an error that maps to a fixed API response (no underlying cause).
func New(code string, status int, message string) *Error {
	return &Error{Code: code, Status: status, Message: message}
}

// Wrap wraps an underlying error with a stable client-facing code; Err is not exposed in JSON.
func Wrap(err error, code string, status int, message string) *Error {
	return &Error{Code: code, Status: status, Message: message, Err: err}
}
