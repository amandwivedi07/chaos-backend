// Package errors defines the reusable application error system.
// Services return *AppError; handlers translate them to HTTP via response pkg.
// This keeps HTTP status codes out of the service layer.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Kind classifies an error independently of transport.
type Kind string

const (
	KindBadRequest   Kind = "BAD_REQUEST"
	KindUnauthorized Kind = "UNAUTHORIZED"
	KindForbidden    Kind = "FORBIDDEN"
	KindNotFound     Kind = "NOT_FOUND"
	KindConflict     Kind = "CONFLICT"
	KindValidation   Kind = "VALIDATION_ERROR"
	KindDatabase     Kind = "DATABASE_ERROR"
	KindInternal     Kind = "INTERNAL_SERVER_ERROR"
	KindRateLimited  Kind = "RATE_LIMITED"
	// KindUnavailable covers a dependency we do not control being off or
	// unhappy — never our own bug, so it must not read as a 500.
	KindUnavailable Kind = "UNAVAILABLE"
)

// AppError is the single error type crossing layer boundaries.
type AppError struct {
	Kind    Kind
	Message string            // safe for clients
	Fields  map[string]string // field-wise validation errors
	Err     error             // wrapped cause (never sent to clients)
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

func (e *AppError) Unwrap() error { return e.Err }

// HTTPStatus maps the kind to a status code — the ONLY place this mapping lives.
func (e *AppError) HTTPStatus() int {
	switch e.Kind {
	case KindBadRequest, KindValidation:
		return http.StatusBadRequest
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	case KindRateLimited:
		return http.StatusTooManyRequests
	case KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// Constructors — one per kind, so call sites read like prose.

func BadRequest(msg string) *AppError   { return &AppError{Kind: KindBadRequest, Message: msg} }
func Unauthorized(msg string) *AppError { return &AppError{Kind: KindUnauthorized, Message: msg} }
func Forbidden(msg string) *AppError    { return &AppError{Kind: KindForbidden, Message: msg} }
func NotFound(msg string) *AppError     { return &AppError{Kind: KindNotFound, Message: msg} }
func Conflict(msg string) *AppError     { return &AppError{Kind: KindConflict, Message: msg} }
func RateLimited(msg string) *AppError  { return &AppError{Kind: KindRateLimited, Message: msg} }
func Unavailable(msg string) *AppError  { return &AppError{Kind: KindUnavailable, Message: msg} }

func Validation(fields map[string]string) *AppError {
	return &AppError{Kind: KindValidation, Message: "Validation failed", Fields: fields}
}

func Database(op string, err error) *AppError {
	return &AppError{Kind: KindDatabase, Message: "A database error occurred", Err: fmt.Errorf("%s: %w", op, err)}
}

func Internal(err error) *AppError {
	return &AppError{Kind: KindInternal, Message: "Something went wrong", Err: err}
}

// From extracts an *AppError, wrapping unknown errors as internal.
func From(err error) *AppError {
	var app *AppError
	if errors.As(err, &app) {
		return app
	}
	return Internal(err)
}
