// Package feishu 提供飞书开放平台 API 客户端，封装 token 管理与文档内容拉取。
package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"local/rag-project/internal/framework/exception"
)

// DocumentFetcher 飞书文档拉取接口，方便测试替换。
type DocumentFetcher interface {
	FetchDocumentContent(ctx context.Context, documentID string) ([]byte, error)
}

const (
	feishuBaseURL            = "https://open.feishu.cn/open-apis"
	tenantAccessTokenPath     = "/auth/v3/tenant_access_token/internal"
	docxRawContentPath        = "/docx/v1/documents/%s/raw_content"
	defaultFeishuTimeout      = 30 * time.Second
	tokenExpireBuffer         = 60 * time.Second // token 提前刷新窗口
)

// Client 飞书 API 客户端，自动管理 tenant_access_token 缓存与刷新。
type Client struct {
	appID       string
	appSecret   string
	httpClient  *http.Client
	baseURL     string

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// NewClient 创建飞书 API 客户端。
func NewClient(appID, appSecret string) *Client {
	return &Client{
		appID:      appID,
		appSecret:  appSecret,
		httpClient: &http.Client{Timeout: defaultFeishuTimeout},
		baseURL:    feishuBaseURL,
	}
}

// FetchDocumentContent 获取飞书 Docx 文档的纯文本内容。
func (c *Client) FetchDocumentContent(ctx context.Context, documentID string) ([]byte, error) {
	if c == nil {
		return nil, exception.NewServiceException("feishu client is required", nil)
	}
	documentID = strings.TrimSpace(documentID)
	if documentID == "" {
		return nil, exception.NewClientException("feishu document id is required", nil)
	}

	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf(docxRawContentPath, documentID)
	body, err := c.doGet(ctx, path, token)
	if err != nil {
		return nil, err
	}

	// 解析飞书 raw_content 响应
	var rawResp feishuRawContentResponse
	if err := json.Unmarshal(body, &rawResp); err != nil {
		return nil, exception.NewServiceException("failed to parse feishu document response", err)
	}
	if rawResp.Code != 0 {
		return nil, exception.NewServiceException(
			fmt.Sprintf("feishu api error: code=%d msg=%s", rawResp.Code, rawResp.Msg),
			nil,
		)
	}

	return []byte(rawResp.Data.Content), nil
}

// getAccessToken 获取 tenant_access_token，自动缓存并在过期前刷新。
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.accessToken != "" && time.Now().Add(tokenExpireBuffer).Before(c.expiresAt) {
		token := c.accessToken
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	return c.fetchAccessToken(ctx)
}

// fetchAccessToken 向飞书鉴权接口请求新 token。
func (c *Client) fetchAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 双重检查：可能在等待锁期间已被其他调用者刷新
	if c.accessToken != "" && time.Now().Add(tokenExpireBuffer).Before(c.expiresAt) {
		return c.accessToken, nil
	}

	reqBody := map[string]string{
		"app_id":     c.appID,
		"app_secret": c.appSecret,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", exception.NewServiceException("failed to marshal feishu auth request", err)
	}

	url := c.baseURL + tenantAccessTokenPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return "", exception.NewServiceException("failed to create feishu auth request", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", exception.NewServiceException("feishu auth request failed", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", exception.NewServiceException("failed to read feishu auth response", err)
	}

	var tokenResp feishuTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", exception.NewServiceException("failed to parse feishu auth response", err)
	}
	if tokenResp.Code != 0 {
		return "", exception.NewServiceException(
			fmt.Sprintf("feishu auth error: code=%d msg=%s", tokenResp.Code, tokenResp.Msg),
			nil,
		)
	}
	if tokenResp.TenantAccessToken == "" {
		return "", exception.NewServiceException("feishu auth returned empty access token", nil)
	}

	c.accessToken = tokenResp.TenantAccessToken
	if tokenResp.Expire > 0 {
		c.expiresAt = time.Now().Add(time.Duration(tokenResp.Expire) * time.Second)
	} else {
		c.expiresAt = time.Now().Add(7200 * time.Second) // 默认 2 小时
	}

	return c.accessToken, nil
}

// doGet 发送带 Authorization 头的 GET 请求，返回响应体。
func (c *Client) doGet(ctx context.Context, path, token string) ([]byte, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, exception.NewServiceException("failed to create feishu api request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, exception.NewServiceException("feishu api request failed", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, exception.NewServiceException("failed to read feishu api response", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, exception.NewServiceException(
			fmt.Sprintf("feishu api returned status %d: %s", resp.StatusCode, string(body)),
			nil,
		)
	}

	return body, nil
}

// 从飞书 URL 或纯 ID 中提取 document ID。
// 支持格式：
//   - https://xxx.feishu.cn/docx/ABCD1234
//   - https://xxx.feishu.cn/wiki/ABCD1234
//   - ABCD1234
func ExtractDocumentID(location string) string {
	location = strings.TrimSpace(location)
	if location == "" {
		return ""
	}
	// 纯 ID 格式
	if !strings.Contains(location, "://") {
		return location
	}
	// URL 格式：取路径最后一段作为 document ID
	if idx := strings.LastIndex(location, "/"); idx >= 0 && idx < len(location)-1 {
		lastPart := location[idx+1:]
		// 去除可能的 query string
		if qIdx := strings.Index(lastPart, "?"); qIdx >= 0 {
			lastPart = lastPart[:qIdx]
		}
		return lastPart
	}
	return location
}

// --- 飞书 API 响应结构 ---

type feishuTokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

type feishuRawContentResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Content string `json:"content"`
	} `json:"data"`
}
