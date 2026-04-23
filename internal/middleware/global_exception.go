package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
	validator "github.com/go-playground/validator/v10"

	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/errorcode"
	"local/rag-project/internal/framework/exception"
)

// GlobalExceptionHandler 提供与 Java GlobalExceptionHandler 等价的全局异常捕获逻辑。
// 使用方法：在 Gin 路由初始化时调用 router.Use(framework.GlobalExceptionHandler())
func GlobalExceptionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				var err error
				switch v := rec.(type) {
				case error:
					err = v
				default:
					err = fmt.Errorf("%v", v)
				}

				// 如果请求已经被中断，直接返回 400
				if c.IsAborted() {
					c.Status(http.StatusBadRequest)
					return
				}

				// 请求信息用于日志记录
				reqDump, _ := httputil.DumpRequest(c.Request, false)

				status := http.StatusInternalServerError
				code := errorcode.ServiceError.Code()
				message := "系统内部错误，请联系管理员"

				// 1) 验证错误（对应 MethodArgumentNotValidException）
				var ve validator.ValidationErrors
				if errors.As(err, &ve) && len(ve) > 0 {
					first := ve[0]
					message = fmt.Sprintf("%s 校验失败: %s", first.Field(), first.Tag())
					status = http.StatusBadRequest
					code = errorcode.ClientError.Code()
					fmt.Fprintf(os.Stderr, "[Validation] %s %s %s\n", c.Request.Method, c.Request.URL.String(), message)

					// 2) 上传大小超限（MaxBytesError 或常见错误字符串）
				} else if _, ok := err.(*http.MaxBytesError); ok {
					status = http.StatusBadRequest
					code = errorcode.ClientError.Code()
					message = fmt.Sprintf("上传请求大小超过限制，单次请求最大允许 %s", "100MB")
				} else if strings.Contains(err.Error(), "request body too large") || strings.Contains(err.Error(), "multipart: message too large") {
					status = http.StatusBadRequest
					code = errorcode.ClientError.Code()
					message = "上传文件大小超过限制"

					// 3) 自定义业务异常（对应 AbstractException 及其子类）
				} else {
					var ae *exception.AbstractException
					if errors.As(err, &ae) {
						code = ae.ErrorCode()
						message = ae.ErrorMessage()
						if strings.HasPrefix(code, "A") {
							status = http.StatusBadRequest
						} else {
							status = http.StatusInternalServerError
						}

					} else {
						// 未知异常，打印堆栈便于排查
						stack := debug.Stack()
						fmt.Fprintf(os.Stderr, "[Recovery] panic recovered:\n%v\n%s\n%s\n", err, reqDump, stack)
					}
				}

				// 返回统一格式
				c.JSON(status, convention.Result[any]{
					Code:    code,
					Message: message,
					Data:    nil,
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}
