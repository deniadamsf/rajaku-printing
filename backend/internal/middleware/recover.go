package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/logger"
)

// Recover is a safety-net panic handler — per spec section 22, panic is only
// acceptable for fatal startup bugs, never in normal request flow. If one
// slips through, log the stack and return a generic 500 envelope.
func Recover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.FromContext(c.Request.Context()).
					Error().
					Interface("panic", r).
					Bytes("stack", debug.Stack()).
					Str("path", c.Request.URL.Path).
					Msg("panic recovered")
				httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
			}
		}()
		c.Next()
	}
}
