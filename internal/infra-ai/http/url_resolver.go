package http

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// ModelUrlResolver 模型 URL 解析器
type ModelUrlResolver struct{}

// NewModelUrlResolver 创建 URL 解析器
func NewModelUrlResolver() *ModelUrlResolver {
	return &ModelUrlResolver{}
}

// ResolveURL 解析模型 URL 地址
// 优先级：候选模型 URL > 提供商基础 URL + 端点路径
func (r *ModelUrlResolver) ResolveURL(
	providerURL string,
	endpoints map[string]string,
	candidateURL string,
	capability string,
) (string, error) {
	// 优先使用候选模型的自定义 URL
	if candidateURL != "" {
		return candidateURL, nil
	}

	// 校验提供商 URL
	if providerURL == "" {
		return "", fmt.Errorf("提供商基础 URL 缺失")
	}

	// 获取端点路径
	key := strings.ToLower(capability)
	endpointPath, ok := endpoints[key]
	if !ok || endpointPath == "" {
		return "", fmt.Errorf("提供商端点配置缺失: %s", key)
	}

	// 拼接 URL
	return r.JoinURL(providerURL, endpointPath), nil
}

// JoinURL 拼接基础 URL 和路径
// 智能处理 URL 和路径之间的斜杠，确保拼接结果正确
func (r *ModelUrlResolver) JoinURL(baseUrl, urlPath string) string {
	// 使用 net/url 进行标准处理
	base, err := url.Parse(baseUrl)
	if err != nil {
		// 如果解析失败，使用简单的字符串拼接
		return r.simpleJoin(baseUrl, urlPath)
	}

	// 使用 path.Join 处理路径
	base.Path = path.Join(base.Path, urlPath)
	return base.String()
}

// simpleJoin 简单的 URL 拼接
func (r *ModelUrlResolver) simpleJoin(baseUrl, urlPath string) string {
	if baseUrl == "" {
		return urlPath
	}
	if urlPath == "" {
		return baseUrl
	}

	// 处理斜杠
	if strings.HasSuffix(baseUrl, "/") && strings.HasPrefix(urlPath, "/") {
		return baseUrl + urlPath[1:]
	}
	if !strings.HasSuffix(baseUrl, "/") && !strings.HasPrefix(urlPath, "/") {
		return baseUrl + "/" + urlPath
	}
	return baseUrl + urlPath
}

// ParseURL 解析 URL 并返回各个部分
func (r *ModelUrlResolver) ParseURL(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

// BuildURL 构建 URL，支持查询参数
func (r *ModelUrlResolver) BuildURL(baseUrl string, urlPath string, params map[string]string) (string, error) {
	parsedURL, err := url.Parse(baseUrl)
	if err != nil {
		return "", fmt.Errorf("解析基础 URL 失败: %w", err)
	}

	// 设置路径
	if urlPath != "" {
		parsedURL.Path = path.Join(parsedURL.Path, urlPath)
	}

	// 添加查询参数
	if len(params) > 0 {
		query := parsedURL.Query()
		for key, value := range params {
			query.Set(key, value)
		}
		parsedURL.RawQuery = query.Encode()
	}

	return parsedURL.String(), nil
}
