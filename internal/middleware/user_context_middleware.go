package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/framework/contextx"
)

// todo : 解决用户上下文的提取

// UserLoaderFunc 根据 loginId 加载用户信息（由调用方实现）
type UserLoaderFunc func(loginId string) (*contextx.LoginUser, error)

// LoginIdExtractor 从请求中提取 loginId 的策略（可自定义）
// 例如：从 Header、Cookie 或 JWT 中解析
type LoginIdExtractor func(c *gin.Context) string

// DefaultLoginIdExtractor 示例：优先从 Header `X-Login-Id` 读取
func DefaultLoginIdExtractor(c *gin.Context) string {
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

// UserContextMiddleware 返回一个 Gin 中间件：
// - 跳过 OPTIONS 预检
// - 使用 extractor 获取 loginId
// - 调用 loader 加载用户并注入到请求上下文（contextx.Set）
// - 请求处理完成后清理上下文（contextx.Clear）
func UserContextMiddleware(loader UserLoaderFunc, extractor LoginIdExtractor) gin.HandlerFunc {
	if extractor == nil {
		extractor = DefaultLoginIdExtractor
	}

	return func(c *gin.Context) {
		// 预检请求放行
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		loginId := extractor(c)
		if loginId != "" && loader != nil {
			if user, err := loader(loginId); err == nil && user != nil {
				contextx.Set(c, user)
			}
		}

		// 确保在请求结束时清理上下文
		defer contextx.Clear(c)
		c.Next()
	}
}
