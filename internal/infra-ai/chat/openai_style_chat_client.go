package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"local/rag-project/internal/framework/convention"
	aihttp "local/rag-project/internal/infra-ai/http"
	"local/rag-project/internal/infra-ai/model"
)

type OpenAIStyleChatClient struct {
	provider string

	httpClient   *http.Client
	streamClient *http.Client
	urlResolver  *aihttp.ModelUrlResolver
	respHelper   *aihttp.ResponseHelper

	requireAPIKey bool

	buildHeaders  func(target model.ModelTarget) http.Header
	customizeBody func(body map[string]any, req convention.ChatRequest, target model.ModelTarget)
	parseStream   func(line string, reasoningEnabled bool) (ParsedEvent, error)
}

type openAIStyleUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIStyleChatResponse struct {
	Choices []openAIStyleChatChoice `json:"choices"`
	Usage   *openAIStyleUsage       `json:"usage"`
}

type openAIStyleChatChoice struct {
	Message *openAIStyleChatMessage `json:"message"`
}

type openAIStyleChatMessage struct {
	Content *string `json:"content"`
}

type cancellableStreamHandle struct {
	cancel context.CancelFunc
}

func (h *cancellableStreamHandle) Cancel() {
	if h != nil && h.cancel != nil {
		h.cancel()
	}
}

func NewOpenAIStyleChatClient(provider string, httpClient *http.Client) *OpenAIStyleChatClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	client := &OpenAIStyleChatClient{
		provider:      provider,
		httpClient:    httpClient,
		streamClient:  httpClient,
		urlResolver:   aihttp.NewModelUrlResolver(),
		respHelper:    aihttp.NewResponseHelper(),
		requireAPIKey: true,
		buildHeaders:  defaultOpenAIStyleHeaders,
		customizeBody: defaultOpenAIStyleCustomizeBody,
		parseStream:   ParseOpenAIStyleSseLine,
	}

	return client
}

func defaultOpenAIStyleHeaders(target model.ModelTarget) http.Header {
	return make(http.Header)
}

func defaultOpenAIStyleCustomizeBody(body map[string]any, req convention.ChatRequest, target model.ModelTarget) {
	if req.ThinkingEnabled() {
		body["enable_thinking"] = true
	}
}

func (op *OpenAIStyleChatClient) Provider() string {
	return op.provider
}

func (op *OpenAIStyleChatClient) isReasoningEnabledForStream(req convention.ChatRequest) bool {
	return req.ThinkingEnabled()
}

func buildMessages(req convention.ChatRequest) []map[string]any {
	arr := make([]map[string]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		item := map[string]any{
			"role":    string(msg.Role),
			"content": msg.Content,
		}
		arr = append(arr, item)
	}
	return arr
}

func (op *OpenAIStyleChatClient) buildRequestBody(req convention.ChatRequest, target model.ModelTarget, stream bool) map[string]any {
	body := map[string]any{
		"model":    target.Candidate.Model,
		"messages": buildMessages(req),
	}

	if stream {
		body["stream"] = true
	}
	if req.HasTemperature() {
		body["temperature"] = *req.Temperature
	}
	if req.HasTopP() {
		body["top_p"] = *req.TopP
	}
	if req.HasTopK() {
		body["top_k"] = *req.TopK
	}
	if req.HasMaxTokens() {
		body["max_tokens"] = *req.MaxTokens
	}
	if req.JSONModeEnabled() {
		body["response_format"] = map[string]string{"type": "json_object"}
	}

	if op != nil && op.customizeBody != nil {
		op.customizeBody(body, req, target)
	}
	return body
}

func (op *OpenAIStyleChatClient) extractContent(resp openAIStyleChatResponse) (string, error) {
	if len(resp.Choices) == 0 {
		return "", aihttp.NewModelClientException(
			fmt.Sprintf("%s response missing choices", op.provider),
			aihttp.ErrorTypeInvalidResponse,
			0,
			nil,
		)
	}

	choice := resp.Choices[0]
	if choice.Message == nil {
		return "", aihttp.NewModelClientException(
			fmt.Sprintf("%s response missing message", op.provider),
			aihttp.ErrorTypeInvalidResponse,
			0,
			nil,
		)
	}
	if choice.Message.Content == nil {
		return "", aihttp.NewModelClientException(
			fmt.Sprintf("%s response missing content", op.provider),
			aihttp.ErrorTypeInvalidResponse,
			0,
			nil,
		)
	}

	return *choice.Message.Content, nil
}

func (op *OpenAIStyleChatClient) Chat(req convention.ChatRequest, target model.ModelTarget) (string, error) {
	content, _, err := op.ChatWithUsage(req, target)
	return content, err
}

func (op *OpenAIStyleChatClient) ChatWithUsage(req convention.ChatRequest, target model.ModelTarget) (string, TokenUsage, error) {
	if err := op.validateRequest(req, target); err != nil {
		return "", TokenUsage{}, err
	}

	body, err := op.marshalRequestBody(req, target, false)
	if err != nil {
		return "", TokenUsage{}, err
	}

	httpReq, err := op.newRequest(context.Background(), target, body, aihttp.MediaTypeJSON)
	if err != nil {
		return "", TokenUsage{}, err
	}

	client := op.effectiveHTTPClient(false)
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", TokenUsage{}, aihttp.NewModelClientException(
			fmt.Sprintf("%s chat request failed", op.provider),
			aihttp.ErrorTypeNetworkError,
			0,
			err,
		)
	}

	if err := op.respHelper.CheckResponse(resp, op.provider); err != nil {
		return "", TokenUsage{}, err
	}

	var result openAIStyleChatResponse
	if err := op.respHelper.ParseJSON(resp.Body, op.provider, &result); err != nil {
		return "", TokenUsage{}, err
	}

	content, err := op.extractContent(result)
	if err != nil {
		return "", TokenUsage{}, err
	}
	return content, extractOpenAIStyleUsage(result.Usage), nil
}

func extractOpenAIStyleUsage(usage *openAIStyleUsage) TokenUsage {
	if usage == nil {
		return TokenUsage{}
	}
	return TokenUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}.Normalized()
}

func (op *OpenAIStyleChatClient) StreamChat(req convention.ChatRequest, callback StreamCallback, target model.ModelTarget) (StreamCancellationHandle, error) {
	if callback == nil {
		return nil, fmt.Errorf(errCallbackNil)
	}
	if err := op.validateRequest(req, target); err != nil {
		return nil, err
	}

	body, err := op.marshalRequestBody(req, target, true)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	httpReq, err := op.newRequest(ctx, target, body, aihttp.MediaTypeSSE)
	if err != nil {
		cancel()
		return nil, err
	}

	handle := &cancellableStreamHandle{cancel: cancel}
	go op.doStream(httpReq, callback, req)
	return handle, nil
}

func (op *OpenAIStyleChatClient) doStream(httpReq *http.Request, callback StreamCallback, req convention.ChatRequest) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("stream goroutine panic recovered: provider=%s panic=%v", op.provider, recovered)
		}
	}()

	dispatcher := newSafeStreamCallbackDispatcher(op.provider, callback)
	client := op.effectiveHTTPClient(true)
	resp, err := client.Do(httpReq)
	if err != nil {
		if httpReq.Context().Err() == nil {
			dispatcher.OnError(aihttp.NewModelClientException(
				fmt.Sprintf("%s stream request failed", op.provider),
				aihttp.ErrorTypeNetworkError,
				0,
				err,
			))
		}
		return
	}
	defer resp.Body.Close()

	if err := op.respHelper.CheckResponse(resp, op.provider); err != nil {
		dispatcher.OnError(err)
		return
	}

	reader := bufio.NewReader(resp.Body)
	completed := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			break
		}

		event, parseErr := op.parseStreamLine(line, op.isReasoningEnabledForStream(req))
		if parseErr != nil {
			dispatcher.OnError(aihttp.NewModelClientException(
				fmt.Sprintf("%s stream parse failed", op.provider),
				aihttp.ErrorTypeInvalidResponse,
				0,
				parseErr,
			))
			return
		}

		if event.HasReasoning() {
			if !dispatcher.OnThinking(event.Reasoning) {
				return
			}
		}
		if event.HasContent() {
			if !dispatcher.OnContent(event.Content) {
				return
			}
		}
		if event.Completed {
			dispatcher.OnComplete()
			completed = true
			return
		}
	}

	if !completed && httpReq.Context().Err() == nil {
		dispatcher.OnError(aihttp.NewModelClientException(
			fmt.Sprintf("%s stream response ended before completion", op.provider),
			aihttp.ErrorTypeInvalidResponse,
			0,
			nil,
		))
	}
}

func (op *OpenAIStyleChatClient) validateRequest(req convention.ChatRequest, target model.ModelTarget) error {
	if op == nil {
		return fmt.Errorf("openai style chat client is nil")
	}
	if err := req.Validate(); err != nil {
		return err
	}
	if op.respHelper == nil {
		op.respHelper = aihttp.NewResponseHelper()
	}
	if op.urlResolver == nil {
		op.urlResolver = aihttp.NewModelUrlResolver()
	}
	if err := op.respHelper.RequireModel(target.Candidate.Model, op.provider); err != nil {
		return err
	}
	if op.requireAPIKey {
		if err := op.respHelper.RequireAPIKey(target.Provider.ApiKey, op.provider); err != nil {
			return err
		}
	}
	return nil
}

func (op *OpenAIStyleChatClient) marshalRequestBody(req convention.ChatRequest, target model.ModelTarget, stream bool) ([]byte, error) {
	body := op.buildRequestBody(req, target, stream)
	data, err := json.Marshal(body)
	if err != nil {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s request marshal failed", op.provider),
			aihttp.ErrorTypeClientError,
			0,
			err,
		)
	}
	return data, nil
}

func (op *OpenAIStyleChatClient) newRequest(ctx context.Context, target model.ModelTarget, body []byte, accept string) (*http.Request, error) {
	url, err := op.urlResolver.ResolveURL(
		target.Provider.Url,
		target.Provider.Endpoints,
		target.Candidate.Url,
		"chat",
	)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set(aihttp.HeaderContentType, aihttp.MediaTypeJSON)
	httpReq.Header.Set("Accept", accept)
	for key, values := range op.defaultHeaders(target) {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}
	if op.requireAPIKey {
		httpReq.Header.Set(aihttp.HeaderAuthorization, "Bearer "+target.Provider.ApiKey)
	}
	return httpReq, nil
}

func (op *OpenAIStyleChatClient) defaultHeaders(target model.ModelTarget) http.Header {
	if op != nil && op.buildHeaders != nil {
		return op.buildHeaders(target)
	}
	return defaultOpenAIStyleHeaders(target)
}

func (op *OpenAIStyleChatClient) effectiveHTTPClient(stream bool) *http.Client {
	if stream {
		if op != nil && op.streamClient != nil {
			return op.streamClient
		}
	}
	if op != nil && op.httpClient != nil {
		return op.httpClient
	}
	return http.DefaultClient
}

func (op *OpenAIStyleChatClient) parseStreamLine(line string, reasoningEnabled bool) (ParsedEvent, error) {
	if op != nil && op.parseStream != nil {
		return op.parseStream(line, reasoningEnabled)
	}
	return ParseOpenAIStyleSseLine(line, reasoningEnabled)
}

type safeStreamCallbackDispatcher struct {
	provider string
	callback StreamCallback
	terminal sync.Once
}

func newSafeStreamCallbackDispatcher(provider string, callback StreamCallback) *safeStreamCallbackDispatcher {
	return &safeStreamCallbackDispatcher{
		provider: provider,
		callback: callback,
	}
}

func (d *safeStreamCallbackDispatcher) OnThinking(content string) bool {
	if err := d.call("thinking", func() {
		d.callback.OnThinking(content)
	}); err != nil {
		d.OnError(err)
		return false
	}
	return true
}

func (d *safeStreamCallbackDispatcher) OnContent(content string) bool {
	if err := d.call("content", func() {
		d.callback.OnContent(content)
	}); err != nil {
		d.OnError(err)
		return false
	}
	return true
}

func (d *safeStreamCallbackDispatcher) OnComplete() {
	d.terminal.Do(func() {
		if err := d.call("complete", func() {
			d.callback.OnComplete()
		}); err != nil {
			d.logCallbackPanic(err)
		}
	})
}

func (d *safeStreamCallbackDispatcher) OnError(err error) {
	if err == nil {
		return
	}
	d.terminal.Do(func() {
		if callbackErr := d.call("error", func() {
			d.callback.OnError(err)
		}); callbackErr != nil {
			d.logCallbackPanic(callbackErr)
		}
	})
}

func (d *safeStreamCallbackDispatcher) call(name string, fn func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = aihttp.NewModelClientException(
				fmt.Sprintf("%s stream callback panic: %s", d.provider, name),
				aihttp.ErrorTypeClientError,
				0,
				fmt.Errorf("%v", recovered),
			)
		}
	}()
	fn()
	return nil
}

func (d *safeStreamCallbackDispatcher) logCallbackPanic(err error) {
	log.Printf("stream callback failed after terminal dispatch: provider=%s err=%v", d.provider, err)
}
