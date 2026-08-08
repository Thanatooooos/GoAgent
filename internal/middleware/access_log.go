package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	fwlog "local/rag-project/internal/framework/log"
)

// AccessLogMiddleware emits one structured completion log for each HTTP request.
func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		path := strings.TrimSpace(c.FullPath())
		if path == "" {
			path = c.Request.URL.Path
		}
		fields := []any{
			"request_id", RequestID(c),
			"method", c.Request.Method,
			"path", path,
			"status_code", c.Writer.Status(),
			"latency_ms", time.Since(startedAt).Milliseconds(),
			"response_size", c.Writer.Size(),
			"client_ip", c.ClientIP(),
		}

		logger := fwlog.FromContext(c.Request.Context())
		switch status := c.Writer.Status(); {
		case status >= 500:
			logger.Errorw("http request completed", fields...)
		case status >= 400:
			logger.Warnw("http request completed", fields...)
		default:
			logger.Infow("http request completed", fields...)
		}
	}
}
