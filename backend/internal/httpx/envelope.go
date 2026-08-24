// Package httpx contains cross-cutting HTTP helpers: response envelope,
// domain-error → HTTP-status mapping, and constructors for typed API errors.
//
// Response envelope (per spec section 22):
//
//	{ "success": bool, "data": <any>, "error": { "code": string, "message": string } }
//
// `data` and `error` are mutually exclusive — never both present in the same
// response. All handlers MUST return responses via OK / Created / Error /
// ErrorFromDomain — never write to gin.Context directly.
package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Success bool          `json:"success"`
	Data    any           `json:"data,omitempty"`
	Error   *ErrorPayload `json:"error,omitempty"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Success: true, Data: data})
}

func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Envelope{Success: true, Data: data})
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Error writes an envelope with the given HTTP status and error code/message.
func Error(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, Envelope{
		Success: false,
		Error:   &ErrorPayload{Code: code, Message: message},
	})
}

// ErrorWithDetails is like Error but includes an optional details payload
// (e.g., field-level validation errors).
func ErrorWithDetails(c *gin.Context, status int, code, message string, details any) {
	c.AbortWithStatusJSON(status, Envelope{
		Success: false,
		Error:   &ErrorPayload{Code: code, Message: message, Details: details},
	})
}

// ParseIntQuery parses an integer query param, returning defaultVal if the
// param is absent/empty. Returns an error (never silently ignored — §22
// dilarang `_ = err`) if the param IS present but isn't a valid integer, so
// callers can reject with 400 instead of e.g. `?page=abc` silently becoming
// page 1. Shared by every handler that parses page/per_page/limit query
// params, so the behaviour stays consistent across the codebase.
func ParseIntQuery(c *gin.Context, key string, defaultVal int) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("query param '%s' harus berupa angka: %w", key, err)
	}
	return n, nil
}

// ErrorFromDomain maps a domain-layer error (see errors.go) to an HTTP
// response. Non-domain errors are returned as 500 with a generic message —
// the caller should have already logged the underlying error with context.
func ErrorFromDomain(c *gin.Context, err error) {
	var de *DomainError
	if errors.As(err, &de) {
		ErrorWithDetails(c, de.HTTPStatus(), de.Code, de.Message, de.Details)
		return
	}
	Error(c, http.StatusInternalServerError, CodeInternal, "internal server error")
}
