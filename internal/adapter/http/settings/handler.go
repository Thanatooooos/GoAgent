package settings

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/middleware"
)

type Handler struct {
	cfg *config.Config
}

func NewHandler(cfg *config.Config) *Handler {
	if cfg == nil {
		cfg = config.Get()
	}
	return &Handler{cfg: cfg}
}

func RegisterRoutes(r gin.IRoutes, cfg *config.Config) {
	handler := NewHandler(cfg)
	r.GET("/rag/settings", handler.GetSystemSettings)
}

type systemSettingsResponse struct {
	Upload uploadSettingsResponse `json:"upload"`
	Rag    ragSettingsResponse    `json:"rag"`
	AI     aiSettingsResponse     `json:"ai"`
}

type uploadSettingsResponse struct {
	MaxFileSize    string `json:"maxFileSize"`
	MaxRequestSize string `json:"maxRequestSize"`
}

type ragSettingsResponse struct {
	Default      ragDefaultResponse      `json:"default"`
	QueryRewrite ragQueryRewriteResponse `json:"queryRewrite"`
	RateLimit    ragRateLimitResponse    `json:"rateLimit"`
	Memory       ragMemoryResponse       `json:"memory"`
}

type ragDefaultResponse struct {
	CollectionName string `json:"collectionName"`
	Dimension      int    `json:"dimension"`
	MetricType     string `json:"metricType"`
}

type ragQueryRewriteResponse struct {
	Enabled bool `json:"enabled"`
}

type ragRateLimitResponse struct {
	Global ragRateLimitGlobalResponse `json:"global"`
}

type ragRateLimitGlobalResponse struct {
	Enabled        bool `json:"enabled"`
	MaxConcurrent  int  `json:"maxConcurrent"`
	MaxWaitSeconds int  `json:"maxWaitSeconds"`
	LeaseSeconds   int  `json:"leaseSeconds"`
	PollIntervalMs int  `json:"pollIntervalMs"`
}

type ragMemoryResponse struct {
	HistoryKeepTurns  int  `json:"historyKeepTurns"`
	SummaryStartTurns int  `json:"summaryStartTurns"`
	SummaryEnabled    bool `json:"summaryEnabled"`
	SummaryMaxChars   int  `json:"summaryMaxChars"`
	TitleMaxLength    int  `json:"titleMaxLength"`
}

type aiSettingsResponse struct {
	Providers map[string]providerResponse `json:"providers"`
	Selection aiSelectionResponse         `json:"selection"`
	Stream    aiStreamResponse            `json:"stream"`
	Chat      modelGroupResponse          `json:"chat"`
	Embedding modelGroupResponse          `json:"embedding"`
	Rerank    modelGroupResponse          `json:"rerank"`
}

type providerResponse struct {
	URL       string            `json:"url"`
	Endpoints map[string]string `json:"endpoints"`
}

type aiSelectionResponse struct {
	FailureThreshold int   `json:"failureThreshold"`
	OpenDurationMs   int64 `json:"openDurationMs"`
}

type aiStreamResponse struct {
	MessageChunkSize int `json:"messageChunkSize"`
}

type modelGroupResponse struct {
	DefaultModel      string                   `json:"defaultModel,omitempty"`
	DeepThinkingModel string                   `json:"deepThinkingModel,omitempty"`
	Candidates        []modelCandidateResponse `json:"candidates"`
}

type modelCandidateResponse struct {
	ID               string `json:"id"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	URL              string `json:"url,omitempty"`
	Dimension        int    `json:"dimension,omitempty"`
	Priority         int    `json:"priority,omitempty"`
	Enabled          *bool  `json:"enabled,omitempty"`
	SupportsThinking *bool  `json:"supportsThinking,omitempty"`
}

func (h *Handler) GetSystemSettings(c *gin.Context) {
	cfg := h.cfg
	if cfg == nil {
		cfg = config.Get()
	}
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, convention.Result[any]{
			Code:      "500",
			Message:   "config not loaded",
			RequestID: middleware.RequestID(c),
			Data:      nil,
		})
		return
	}

	c.JSON(http.StatusOK, convention.Result[systemSettingsResponse]{
		Code:      "0",
		RequestID: middleware.RequestID(c),
		Data:      toSystemSettingsResponse(cfg),
	})
}

func toSystemSettingsResponse(cfg *config.Config) systemSettingsResponse {
	return systemSettingsResponse{
		Upload: uploadSettingsResponse{
			MaxFileSize:    cfg.Spring.Servlet.Multipart.MaxFileSize,
			MaxRequestSize: cfg.Spring.Servlet.Multipart.MaxRequestSize,
		},
		Rag: ragSettingsResponse{
			Default: ragDefaultResponse{
				CollectionName: cfg.Rag.Default.CollectionName,
				Dimension:      cfg.Rag.Default.Dimension,
				MetricType:     cfg.Rag.Default.MetricType,
			},
			QueryRewrite: ragQueryRewriteResponse{
				Enabled: cfg.Rag.QueryRewrite.Enabled,
			},
			RateLimit: ragRateLimitResponse{
				Global: ragRateLimitGlobalResponse{
					Enabled:        cfg.Rag.RateLimit.Global.Enabled,
					MaxConcurrent:  cfg.Rag.RateLimit.Global.MaxConcurrent,
					MaxWaitSeconds: cfg.Rag.RateLimit.Global.MaxWaitSeconds,
					LeaseSeconds:   cfg.Rag.RateLimit.Global.LeaseSeconds,
					PollIntervalMs: cfg.Rag.RateLimit.Global.PollIntervalMs,
				},
			},
			Memory: ragMemoryResponse{
				HistoryKeepTurns:  cfg.Rag.Memory.HistoryKeepTurns,
				SummaryStartTurns: cfg.Rag.Memory.SummaryStartTurns,
				SummaryEnabled:    cfg.Rag.Memory.SummaryEnabled,
				SummaryMaxChars:   cfg.Rag.Memory.SummaryMaxChars,
				TitleMaxLength:    cfg.Rag.Memory.TitleMaxLength,
			},
		},
		AI: aiSettingsResponse{
			Providers: toProviderResponses(cfg.AI.Providers),
			Selection: aiSelectionResponse{
				FailureThreshold: cfg.AI.Selection.FailureThreshold,
				OpenDurationMs:   cfg.AI.Selection.OpenDurationMs,
			},
			Stream: aiStreamResponse{
				MessageChunkSize: cfg.AI.Stream.MessageChunkSize,
			},
			Chat:      toModelGroupResponse(cfg.AI.Chat),
			Embedding: toModelGroupResponse(cfg.AI.Embedding),
			Rerank:    toModelGroupResponse(cfg.AI.Rerank),
		},
	}
}

func toProviderResponses(providers map[string]config.ProviderConfig) map[string]providerResponse {
	result := make(map[string]providerResponse, len(providers))
	for key, provider := range providers {
		result[key] = providerResponse{
			URL:       provider.Url,
			Endpoints: provider.Endpoints,
		}
	}
	return result
}

func toModelGroupResponse(group config.ModelGroup) modelGroupResponse {
	result := modelGroupResponse{
		DefaultModel:      group.DefaultModel,
		DeepThinkingModel: group.DeepThinkingModel,
		Candidates:        make([]modelCandidateResponse, 0, len(group.Candidates)),
	}
	for _, candidate := range group.Candidates {
		result.Candidates = append(result.Candidates, modelCandidateResponse{
			ID:               candidate.Id,
			Provider:         candidate.Provider,
			Model:            candidate.Model,
			URL:              candidate.Url,
			Dimension:        candidate.DimensionInt(0),
			Priority:         candidate.Priority,
			Enabled:          candidate.Enabled,
			SupportsThinking: candidate.SupportsThinking,
		})
	}
	return result
}
