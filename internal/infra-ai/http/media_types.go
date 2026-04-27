package aihttp

// HTTP 媒体类型常量
const (
	MediaTypeJSON = "application/json; charset=utf-8"
	MediaTypeText = "text/plain; charset=utf-8"
	MediaTypeSSE  = "text/event-stream"
)

// Content-Type headers
const (
	HeaderContentType   = "Content-Type"
	HeaderAuthorization = "Authorization"
	HeaderUserAgent     = "User-Agent"
)
