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

type BaiLianRerankClient struct {
	httpClient  *http.Client
	urlResolver *aihttp.ModelUrlResolver
	respHelper  *aihttp.ResponseHelper
}

type baiLianRerankResponse struct {
	Output *baiLianRerankOutput `json:"output"`
}

type baiLianRerankOutput struct {
	Results []baiLianRerankResult `json:"results"`
}

type baiLianRerankResult struct {
	Index          int      `json:"index"`
	RelevanceScore *float32 `json:"relevance_score"`
}

func NewBaiLianRerankClient(httpClient *http.Client) *BaiLianRerankClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &BaiLianRerankClient{
		httpClient:  httpClient,
		urlResolver: aihttp.NewModelUrlResolver(),
		respHelper:  aihttp.NewResponseHelper(),
	}
}

func (b *BaiLianRerankClient) Provider() string {
	return "bailian"
}

func (b *BaiLianRerankClient) Rerank(query string, candidates []convention.RetrievedChunk, topN int, target model.ModelTarget) ([]convention.RetrievedChunk, error) {
	if len(candidates) == 0 {
		return []convention.RetrievedChunk{}, nil
	}

	dedup := dedupChunks(candidates)
	if topN <= 0 || len(dedup) <= topN {
		return dedup, nil
	}

	if err := b.respHelper.RequireAPIKey(target.Provider.ApiKey, b.Provider()); err != nil {
		return nil, err
	}
	if err := b.respHelper.RequireModel(target.Candidate.Model, b.Provider()); err != nil {
		return nil, err
	}

	body := map[string]any{
		"model": target.Candidate.Model,
		"input": map[string]any{
			"query": query,
			"documents": func() []string {
				docs := make([]string, 0, len(dedup))
				for _, item := range dedup {
					docs = append(docs, item.Text)
				}
				return docs
			}(),
		},
		"parameters": map[string]any{
			"top_n":            topN,
			"return_documents": true,
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s rerank request marshal failed", b.Provider()),
			aihttp.ErrorTypeClientError,
			0,
			err,
		)
	}

	url, err := b.urlResolver.ResolveURL(target.Provider.Url, target.Provider.Endpoints, target.Candidate.Url, "rerank")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set(aihttp.HeaderContentType, aihttp.MediaTypeJSON)
	req.Header.Set("Accept", aihttp.MediaTypeJSON)
	req.Header.Set(aihttp.HeaderAuthorization, "Bearer "+target.Provider.ApiKey)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s rerank request failed", b.Provider()),
			aihttp.ErrorTypeNetworkError,
			0,
			err,
		)
	}

	if err := b.respHelper.CheckResponse(resp, b.Provider()); err != nil {
		return nil, err
	}

	var parsed baiLianRerankResponse
	if err := b.respHelper.ParseJSON(resp.Body, b.Provider(), &parsed); err != nil {
		return nil, err
	}

	return b.extractResults(parsed, dedup, topN)
}

func (b *BaiLianRerankClient) extractResults(resp baiLianRerankResponse, candidates []convention.RetrievedChunk, topN int) ([]convention.RetrievedChunk, error) {
	if resp.Output == nil {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s rerank response missing output", b.Provider()),
			aihttp.ErrorTypeInvalidResponse,
			0,
			nil,
		)
	}
	if len(resp.Output.Results) == 0 {
		return nil, aihttp.NewModelClientException(
			fmt.Sprintf("%s rerank response missing results", b.Provider()),
			aihttp.ErrorTypeInvalidResponse,
			0,
			nil,
		)
	}

	reranked := make([]convention.RetrievedChunk, 0, topN)
	added := make(map[string]struct{}, topN)
	for _, item := range resp.Output.Results {
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

func dedupChunks(candidates []convention.RetrievedChunk) []convention.RetrievedChunk {
	result := make([]convention.RetrievedChunk, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, item := range candidates {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		result = append(result, item)
	}
	return result
}
