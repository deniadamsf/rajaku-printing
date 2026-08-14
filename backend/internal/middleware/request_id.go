package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/logger"
)

const (
	HeaderRequestID = "X-Request-ID"
	ContextKeyReqID = "request_id"
)

// RequestID attaches an incoming X-Request-ID (or generates one) to the gin
// context, echoes it back in the response header, and enriches the context
// logger with `request_id` so downstream logs carry it automatically.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader(HeaderRequestID)
		if reqID == "" {
			reqID = uuid.NewString()
		}
		c.Set(ContextKeyReqID, reqID)
		c.Writer.Header().Set(HeaderRequestID, reqID)

		ctx := logger.WithRequestID(c.Request.Context(), reqID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
