package httpx

import (
	"fmt"
	"net/http"
)

// Error code constants — stable string identifiers exposed to API clients.
// Add new codes here when introducing new failure classes; do NOT invent
// ad-hoc strings in handlers/services.
const (
	CodeBadRequest      = "BAD_REQUEST"
	CodeValidation      = "VALIDATION_FAILED"
	CodeUnauthorized    = "UNAUTHORIZED"
	CodeForbidden       = "FORBIDDEN"
	CodeNotFound        = "NOT_FOUND"
	CodeConflict        = "CONFLICT"
	CodeRateLimited     = "RATE_LIMITED"
	CodeUnprocessable   = "UNPROCESSABLE"
	CodeInternal        = "INTERNAL_ERROR"
	CodeServiceUnavail  = "SERVICE_UNAVAILABLE"
)

// DomainError is the shared error type crossing service → handler layers.
// Services return DomainError for expected failures (validation, not-found,
// conflict); unexpected errors bubble up as plain errors and become 500.
type DomainError struct {
	Code    string
	Message string
	Details any
	Cause   error
}

func (e *DomainError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *DomainError) Unwrap() error { return e.Cause }

func (e *DomainError) HTTPStatus() int {
	switch e.Code {
	case CodeBadRequest, CodeValidation:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeUnprocessable:
		return http.StatusUnprocessableEntity
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeServiceUnavail:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// Constructors — prefer these over building DomainError literals inline.

func NewBadRequest(msg string, cause error) *DomainError {
	return &DomainError{Code: CodeBadRequest, Message: msg, Cause: cause}
}

func NewValidation(msg string, details any) *DomainError {
	return &DomainError{Code: CodeValidation, Message: msg, Details: details}
}

func NewNotFound(msg string) *DomainError {
	return &DomainError{Code: CodeNotFound, Message: msg}
}

func NewConflict(msg string) *DomainError {
	return &DomainError{Code: CodeConflict, Message: msg}
}

func NewUnauthorized(msg string) *DomainError {
	return &DomainError{Code: CodeUnauthorized, Message: msg}
}

func NewForbidden(msg string) *DomainError {
	return &DomainError{Code: CodeForbidden, Message: msg}
}
