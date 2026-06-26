package rerank

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"local/rag-project/internal/framework/convention"
	aihttp "local/rag-project/internal/infra-ai/http"
	"local/rag-project/internal/infra-ai/model"
)

type OpenAIStyleRerankClient struct {
	provider string

	httpClient  *http.Client
	urlResolver *aihttp.ModelUrlResolver
	respHelper  *aihttp.ResponseHelper

	requireAPIKey bool
	buildHeaders  func(target model.ModelTarget) http.Header
	customizeBody func(body map[string]any, query string, topN int, target model.ModelTarget)
}

type openAIStyleRerankResponse struct {
	Error   *openAIStyleRerankError    `json:"error"`
	Results []openAIStyleRerankResult  `json:"results"`
	Output  *openAIStyleRerankEnvelope `json:"output"`
}

type openAIStyleRerankError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type openAIStyleRerankEnvelope struct {
	Results []openAIStyleRerankResult `json:"results"`
}

type openAIStyleRerankResult struct {
	Index          int      `json:"index"`
	RelevanceScore *float32 `json:"relevance_score"`
}

func NewOpenAIStyleRerankClient(provider string, httpClient *http.Client) *OpenAIStyleRerankClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAIStyleRerankClient{
		provider:      provider,
		httpClient:    httpClient,
		urlResolver:   aihttp.NewModelUrlResolver(),
		respHelper:    aihttp.NewResponseHelper(),
		requireAPIKey: true,
		buildHeaders:  defaultOpenAIStyleRerankHeaders,
		customizeBody: defaultOpenAIStyleRerankBodyCustomizer,
	}
}

func defaultOpenAIStyleRerankHeaders(target model.ModelTarget) http.Header {
	return make(http.Header)
}

func defaultOpenAIStyleRerankBodyCustomizer(body map[string]any, query string, topN int, target model.ModelTarget) {
	body["query"] = query
	body["top_n"] = topN
}

func (op *OpenAIStyleRerankClient) Provider() string {
	return op.provider
}

func (op *OpenAIStyleRerankClient) Rerank(query string, candidates []convention.RetrievedChunk, topN int, target model.ModelTarget) ([]convention.RetrievedChunk, error) {
	dedup := dedupChunks(candidates)
	if len(dedup) == 0 {
		return []convention.RetrievedChunk{}, nil
	}
	if topN <= 0 || len(dedup) <= topN {
		return dedup, nil
	}
	if err := op.validateTarget(target); err != nil {
		return nil, err
	}

	payload := op.buildRequestBody(query, dedup, topN, target)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s rerank request marshal failed", op.provider),
			aihttp.ErrorTypeClientError,
			0,
			err,
		)
	}

	url, err := op.urlResolver.ResolveURL(target.Provider.Url, target.Provider.Endpoints, target.Candidate.Url, "rerank")
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
			fmt.Sprintf("%s rerank request failed", op.provider),
			aihttp.ErrorTypeNetworkError,
			0,
			err,
		)
	}
	if err := op.respHelper.CheckResponse(resp, op.provider); err != nil {
		return nil, err
	}

	var parsed openAIStyleRerankResponse
	if err := op.respHelper.ParseJSON(resp.Body, op.provider, &parsed); err != nil {
		return nil, err
	}
	return op.extractResults(parsed, dedup, topN)
}

func (op *OpenAIStyleRerankClient) buildRequestBody(query string, candidates []convention.RetrievedChunk, topN int, target model.ModelTarget) map[string]any {
	documents := make([]string, 0, len(candidates))
	for _, item := range candidates {
		documents = append(documents, item.Text)
	}

	body := map[string]any{
		"model":     target.Candidate.Model,
		"documents": documents,
	}
	if op.customizeBody != nil {
		op.customizeBody(body, query, topN, target)
	}
	return body
}

func (op *OpenAIStyleRerankClient) extractResults(resp openAIStyleRerankResponse, candidates []convention.RetrievedChunk, topN int) ([]convention.RetrievedChunk, error) {
	if resp.Error != nil {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s rerank error: %s - %s", op.provider, resp.Error.Code, resp.Error.Message),
			aihttp.ErrorTypeProviderError,
			0,
			nil,
		)
	}

	results := resp.Results
	if len(results) == 0 && resp.Output != nil {
		results = resp.Output.Results
	}
	if len(results) == 0 {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s rerank response missing results", op.provider),
			aihttp.ErrorTypeInvalidResponse,
			0,
			nil,
		)
	}

	reranked := make([]convention.RetrievedChunk, 0, topN)
	added := make(map[string]struct{}, topN)
	for _, item := range results {
		if item.Index < 0 || item.Index >= len(candidates) {
			continue
		}
		source := candidates[item.Index]
		if item.RelevanceScore != nil {
			source.Score = *item.RelevanceScore
		}
		reranked = append(reranked, source)
		added[source.ID] = struct{}{}
		if len(reranked) >= topN {
			break
		}
	}

	if len(reranked) < topN {
		for _, item := range candidates {
			if _, ok := added[item.ID]; ok {
				continue
			}
			reranked = append(reranked, item)
			if len(reranked) >= topN {
				break
			}
		}
	}

	return reranked, nil
}

func (op *OpenAIStyleRerankClient) validateTarget(target model.ModelTarget) error {
	if op == nil {
		return fmt.Errorf("openai style rerank client is nil")
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

func (op *OpenAIStyleRerankClient) defaultHeaders(target model.ModelTarget) http.Header {
	if op.buildHeaders != nil {
		return op.buildHeaders(target)
	}
	return defaultOpenAIStyleRerankHeaders(target)
}
