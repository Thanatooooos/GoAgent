package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const (
	requestIDKey    = "request_id"
	requestIDHeader = "X-Request-Id"
)

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(requestIDHeader)
		if requestID == "" {
			requestID = newRequestID()
		}
		c.Set(requestIDKey, requestID)
		c.Header(requestIDHeader, requestID)
		c.Next()
	}
}

func RequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(requestIDKey); ok {
		if requestID, ok := v.(string); ok {
			return requestID
		}
	}
	return c.GetHeader(requestIDHeader)
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
