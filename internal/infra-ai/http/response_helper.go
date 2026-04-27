package aihttp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ResponseHelper HTTP 响应处理工具
type ResponseHelper struct{}

// NewResponseHelper 创建响应处理工具
func NewResponseHelper() *ResponseHelper {
	return &ResponseHelper{}
}

// ReadBody 读取响应体原始字符串
func (h *ResponseHelper) ReadBody(body io.ReadCloser) (string, error) {
	if body == nil {
		return "", nil
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("读取响应体失败: %w", err)
	}
	return string(data), nil
}

// ParseJSON 将响应体解析为 JSON
func (h *ResponseHelper) ParseJSON(body io.ReadCloser, label string, v interface{}) error {
	if body == nil {
		return NewModelClientException(
			label+" 响应为空",
			ErrorTypeInvalidResponse,
			0,
			nil,
		)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return NewModelClientException(
			label+" 读取响应失败",
			ErrorTypeNetworkError,
			0,
			err,
		)
	}

	if err := json.Unmarshal(data, v); err != nil {
		return NewModelClientException(
			fmt.Sprintf("%s JSON 解析失败: %s", label, string(data)),
			ErrorTypeInvalidResponse,
			0,
			err,
		)
	}

	return nil
}

// ParseJSONMap 将响应体解析为 map[string]interface{}
func (h *ResponseHelper) ParseJSONMap(body io.ReadCloser, label string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := h.ParseJSON(body, label, &result)
	return result, err
}

// CheckResponse 检查 HTTP 响应状态
func (h *ResponseHelper) CheckResponse(resp *http.Response, label string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := h.ReadBody(resp.Body)
	return NewModelClientException(
		fmt.Sprintf("%s 请求失败: status=%d, body=%s", label, resp.StatusCode, body),
		FromHttpStatus(resp.StatusCode),
		resp.StatusCode,
		nil,
	)
}

// RequireProvider 校验并返回提供商配置
func (h *ResponseHelper) RequireProvider(provider interface{}, label string) error {
	if provider == nil {
		return fmt.Errorf("%s 提供商配置缺失", label)
	}
	return nil
}

// RequireAPIKey 校验提供商 API 密钥
func (h *ResponseHelper) RequireAPIKey(apiKey string, label string) error {
	if apiKey == "" {
		return fmt.Errorf("%s API密钥缺失", label)
	}
	return nil
}

// RequireModel 校验并返回模型名称
func (h *ResponseHelper) RequireModel(model string, label string) error {
	if model == "" {
		return fmt.Errorf("%s 模型名称缺失", label)
	}
	return nil
}
