package middleware

import (
	"github.com/gin-gonic/gin"

	fwlog "local/rag-project/internal/framework/log"
)

// LogContextMiddleware binds request_id onto the request context logger.
func LogContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := RequestID(c)
		if requestID != "" {
			ctx := fwlog.NewContext(c.Request.Context(), "request_id", requestID)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}
