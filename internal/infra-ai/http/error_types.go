package aihttp

// ModelClientErrorType 模型客户端错误类型
type ModelClientErrorType int

const (
	// ErrorTypeUnauthorized 未授权错误 - 认证失败或令牌无效
	ErrorTypeUnauthorized ModelClientErrorType = iota
	// ErrorTypeRateLimited 速率限制错误 - 请求频率超过限制
	ErrorTypeRateLimited
	// ErrorTypeServerError 服务器错误 - 模型服务端内部错误
	ErrorTypeServerError
	// ErrorTypeClientError 客户端错误 - 请求参数或格式错误
	ErrorTypeClientError
	// ErrorTypeNetworkError 网络错误 - 网络连接或超时问题
	ErrorTypeNetworkError
	// ErrorTypeInvalidResponse 无效响应 - 模型返回的响应格式不正确
	ErrorTypeInvalidResponse
	// ErrorTypeProviderError 供应商错误 - 模型提供商服务错误
	ErrorTypeProviderError
)

// String 返回错误类型的字符串表示
func (e ModelClientErrorType) String() string {
	switch e {
	case ErrorTypeUnauthorized:
		return "UNAUTHORIZED"
	case ErrorTypeRateLimited:
		return "RATE_LIMITED"
	case ErrorTypeServerError:
		return "SERVER_ERROR"
	case ErrorTypeClientError:
		return "CLIENT_ERROR"
	case ErrorTypeNetworkError:
		return "NETWORK_ERROR"
	case ErrorTypeInvalidResponse:
		return "INVALID_RESPONSE"
	case ErrorTypeProviderError:
		return "PROVIDER_ERROR"
	default:
		return "UNKNOWN"
	}
}

// FromHttpStatus 根据 HTTP 状态码推断错误类型
func FromHttpStatus(status int) ModelClientErrorType {
	if status == 401 || status == 403 {
		return ErrorTypeUnauthorized
	}
	if status == 429 {
		return ErrorTypeRateLimited
	}
	if status >= 500 {
		return ErrorTypeServerError
	}
	return ErrorTypeClientError
}
