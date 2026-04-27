package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/errorcode"
	"local/rag-project/internal/framework/exception"
	fwlog "local/rag-project/internal/framework/log"
)

type UserLoaderFunc func(loginID string) (*contextx.LoginUser, error)

type LoginIDExtractor func(c *gin.Context) string

func DefaultLoginIDExtractor(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if id := c.GetHeader("X-Login-Id"); id != "" {
		return id
	}
	if cookie, err := c.Cookie("login_id"); err == nil {
		return cookie
	}
	return ""
}

func UserContextMiddleware(loader UserLoaderFunc, extractor LoginIDExtractor) gin.HandlerFunc {
	if extractor == nil {
		extractor = DefaultLoginIDExtractor
	}

	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		defer contextx.Clear(c)

		loginID := extractor(c)
		if loginID == "" || loader == nil {
			c.Next()
			return
		}

		user, err := loader(loginID)
		if err != nil {
			fwlog.Errorf("load login user failed: loginID=%s error=%v", loginID, err)
			_ = c.Error(exception.NewServiceException("load login user failed", err))
			c.Abort()
			return
		}
		if user == nil {
			_ = c.Error(exception.NewClientException("invalid login identity", nil))
			c.Abort()
			return
		}

		contextx.Set(c, user)
		c.Next()
	}
}

func RequireLogin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if contextx.Get(c) == nil {
			abortWithError(c, http.StatusUnauthorized, errorcode.ClientError.Code(), "unauthorized")
			return
		}
		c.Next()
	}
}

func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := contextx.Get(c)
		if user == nil {
			abortWithError(c, http.StatusUnauthorized, errorcode.ClientError.Code(), "unauthorized")
			return
		}
		if user.Role != role {
			abortWithError(c, http.StatusForbidden, errorcode.ClientError.Code(), "forbidden")
			return
		}
		c.Next()
	}
}
