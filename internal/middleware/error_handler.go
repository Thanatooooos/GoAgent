package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
	validator "github.com/go-playground/validator/v10"

	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/errorcode"
	fwlog "local/rag-project/internal/framework/log"
)

func ErrorHandlerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				err, ok := rec.(error)
				if !ok {
					err = fmt.Errorf("%v", rec)
				}

				reqDump, _ := httputil.DumpRequest(c.Request, false)
				fwlog.Errorf("panic recovered: error=%v request=%s stack=%s", err, string(reqDump), string(debug.Stack()))
				writeError(c, err)
				c.Abort()
			}
		}()

		c.Next()

		if c.Writer.Written() || len(c.Errors) == 0 {
			return
		}
		writeError(c, c.Errors.Last().Err)
	}
}

func GlobalExceptionHandler() gin.HandlerFunc {
	return ErrorHandlerMiddleware()
}

func writeError(c *gin.Context, err error) {
	status, code, message := classifyError(err)
	abortWithError(c, status, code, message)
}

func abortWithError(c *gin.Context, status int, code string, message string) {
	if c.Writer.Written() {
		return
	}
	c.JSON(status, convention.Result[any]{
		Code:      code,
		Message:   message,
		RequestID: RequestID(c),
		Data:      nil,
	})
	c.Abort()
}

func classifyError(err error) (int, string, string) {
	status := http.StatusInternalServerError
	code := errorcode.ServiceError.Code()
	message := "internal server error"

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) && len(validationErrors) > 0 {
		first := validationErrors[0]
		return http.StatusBadRequest, errorcode.ClientError.Code(), fmt.Sprintf("%s validation failed: %s", first.Field(), first.Tag())
	}

	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return http.StatusBadRequest, errorcode.ClientError.Code(), "request body too large"
	}

	if err != nil {
		errText := err.Error()
		if strings.Contains(errText, "request body too large") || strings.Contains(errText, "multipart: message too large") {
			return http.StatusBadRequest, errorcode.ClientError.Code(), "request body too large"
		}
	}

	var appErr interface {
		ErrorCode() string
		ErrorMessage() string
	}
	if errors.As(err, &appErr) {
		code = appErr.ErrorCode()
		message = appErr.ErrorMessage()
		switch {
		case strings.HasPrefix(code, "A"):
			status = http.StatusBadRequest
		case strings.HasPrefix(code, "C"):
			status = http.StatusBadGateway
		default:
			status = http.StatusInternalServerError
		}
	}

	return status, code, message
}
