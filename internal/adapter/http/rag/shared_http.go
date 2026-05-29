package rag

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/middleware"
)

// requireLoginUser 提取当前登录用户。
func requireLoginUser(c *gin.Context) *contextx.LoginUser {
	user := contextx.Get(c)
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		_ = c.Error(exception.NewClientException("unauthorized", nil))
		return nil
	}
	return user
}

// splitCommaValues 按逗号拆分查询参数。
func splitCommaValues(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// parseBool 解析查询参数中的布尔值。
func parseBool(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes"
}

// writeSuccess 输出统一成功响应。
func writeSuccess[T any](c *gin.Context, data T) {
	c.JSON(http.StatusOK, convention.Result[T]{
		Code:      "0",
		RequestID: middleware.RequestID(c),
		Data:      data,
	})
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
