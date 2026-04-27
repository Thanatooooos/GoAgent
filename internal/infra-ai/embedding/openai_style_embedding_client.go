package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	aihttp "local/rag-project/internal/infra-ai/http"
	"local/rag-project/internal/infra-ai/model"
)

type OpenAIStyleEmbeddingClient struct {
	provider string

	httpClient  *http.Client
	urlResolver *aihttp.ModelUrlResolver
	respHelper  *aihttp.ResponseHelper

	requireAPIKey bool
	maxBatchSize  int

	buildHeaders  func(target model.ModelTarget) http.Header
	customizeBody func(body map[string]any, target model.ModelTarget)
}

type openAIStyleEmbeddingResponse struct {
	Error *openAIStyleEmbeddingError `json:"error"`
	Data  []openAIStyleEmbeddingData `json:"data"`
}

type openAIStyleEmbeddingError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type openAIStyleEmbeddingData struct {
	Embedding []float32 `json:"embedding"`
}

func NewOpenAIStyleEmbeddingClient(provider string, httpClient *http.Client) *OpenAIStyleEmbeddingClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAIStyleEmbeddingClient{
		provider:      provider,
		httpClient:    httpClient,
		urlResolver:   aihttp.NewModelUrlResolver(),
		respHelper:    aihttp.NewResponseHelper(),
		requireAPIKey: true,
		buildHeaders:  defaultOpenAIStyleEmbeddingHeaders,
		customizeBody: defaultOpenAIStyleEmbeddingBodyCustomizer,
		maxBatchSize:  0,
	}
}

func defaultOpenAIStyleEmbeddingHeaders(target model.ModelTarget) http.Header {
	return make(http.Header)
}

func defaultOpenAIStyleEmbeddingBodyCustomizer(body map[string]any, target model.ModelTarget) {
	body["encoding_format"] = "float"
	if dim := target.Candidate.DimensionInt(0); dim > 0 {
		body["dimensions"] = dim
	}
}

func (op *OpenAIStyleEmbeddingClient) Provider() string {
	return op.provider
}

func (op *OpenAIStyleEmbeddingClient) Embed(text string, target model.ModelTarget) ([]float32, error) {
	result, err := op.doEmbed([]string{text}, target)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s embedding response missing vectors", op.provider),
			aihttp.ErrorTypeInvalidResponse,
			0,
			nil,
		)
	}
	return result[0], nil
}

func (op *OpenAIStyleEmbeddingClient) EmbedBatch(texts []string, target model.ModelTarget) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	batchSize := op.maxBatchSize
	if batchSize <= 0 || len(texts) <= batchSize {
		return op.doEmbed(texts, target)
	}

	results := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		part, err := op.doEmbed(texts[start:end], target)
		if err != nil {
			return nil, err
		}
		results = append(results, part...)
	}
	return results, nil
}

func (op *OpenAIStyleEmbeddingClient) doEmbed(texts []string, target model.ModelTarget) ([][]float32, error) {
	if err := op.validateTarget(target); err != nil {
		return nil, err
	}

	payload := op.buildRequestBody(texts, target)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s embedding request marshal failed", op.provider),
			aihttp.ErrorTypeClientError,
			0,
			err,
		)
	}

	url, err := op.urlResolver.ResolveURL(target.Provider.Url, target.Provider.Endpoints, target.Candidate.Url, "embedding")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set(aihttp.HeaderContentType, aihttp.MediaTypeJSON)
	req.Header.Set("Accept", aihttp.MediaTypeJSON)
	for key, values := range op.defaultHeaders(target) {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if op.requireAPIKey {
		req.Header.Set(aihttp.HeaderAuthorization, "Bearer "+target.Provider.ApiKey)
	}

	resp, err := op.httpClient.Do(req)
	if err != nil {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s embedding request failed", op.provider),
			aihttp.ErrorTypeNetworkError,
			0,
			err,
		)
	}

	if err := op.respHelper.CheckResponse(resp, op.provider); err != nil {
		return nil, err
	}

	var parsed openAIStyleEmbeddingResponse
	if err := op.respHelper.ParseJSON(resp.Body, op.provider, &parsed); err != nil {
		return nil, err
	}
	return op.extractEmbeddings(parsed)
}

func (op *OpenAIStyleEmbeddingClient) buildRequestBody(texts []string, target model.ModelTarget) map[string]any {
	body := map[string]any{
		"model": target.Candidate.Model,
		"input": texts,
	}
	if op.customizeBody != nil {
		op.customizeBody(body, target)
	}
	return body
}

func (op *OpenAIStyleEmbeddingClient) extractEmbeddings(resp openAIStyleEmbeddingResponse) ([][]float32, error) {
	if resp.Error != nil {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s embedding error: %s - %s", op.provider, resp.Error.Code, resp.Error.Message),
			aihttp.ErrorTypeProviderError,
			0,
			nil,
		)
	}
	if len(resp.Data) == 0 {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s embedding response missing data", op.provider),
			aihttp.ErrorTypeInvalidResponse,
			0,
			nil,
		)
	}

	result := make([][]float32, 0, len(resp.Data))
	for _, item := range resp.Data {
		if len(item.Embedding) == 0 {
			return nil, aihttp.NewModelClientException(
				fmt.Sprintf("%s embedding response missing embedding", op.provider),
				aihttp.ErrorTypeInvalidResponse,
				0,
				nil,
			)
		}
		result = append(result, item.Embedding)
	}
	return result, nil
}

func (op *OpenAIStyleEmbeddingClient) validateTarget(target model.ModelTarget) error {
	if op == nil {
		return fmt.Errorf("openai style embedding client is nil")
	}
	if op.urlResolver == nil {
		op.urlResolver = aihttp.NewModelUrlResolver()
	}
	if op.respHelper == nil {
		op.respHelper = aihttp.NewResponseHelper()
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

func (op *OpenAIStyleEmbeddingClient) defaultHeaders(target model.ModelTarget) http.Header {
	if op.buildHeaders != nil {
		return op.buildHeaders(target)
	}
	return defaultOpenAIStyleEmbeddingHeaders(target)
}
