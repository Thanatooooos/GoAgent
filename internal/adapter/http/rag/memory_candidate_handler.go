package rag

import (
	"math"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/framework/exception"
)

type preferenceCandidateVO struct {
	ID               string  `json:"id,omitempty"`
	ScopeType        string  `json:"scopeType"`
	MemoryType       string  `json:"memoryType"`
	CanonicalKey     string  `json:"canonicalKey"`
	Summary          string  `json:"summary"`
	Content          string  `json:"content"`
	SourceMessageID  string  `json:"sourceMessageId"`
	ExtractionMethod string  `json:"extractionMethod"`
	Confidence       float64 `json:"confidence"`
	Status           string  `json:"status"`
}

func (h *Handler) ListPendingPreferenceCandidates(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	service, ok := h.requirePreferenceCandidateService(c)
	if !ok {
		return
	}
	result, err := service.ListPendingPreferenceCandidates(c.Request.Context(), longtermmemory.ListPreferenceCandidatesInput{
		UserID:   user.UserID,
		Page:     parsePositiveInt(c.Query("current"), 1),
		PageSize: parsePositiveInt(c.Query("size"), longtermmemory.DefaultPreferenceCandidatePageSize),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	records := make([]preferenceCandidateVO, 0, len(result.Items))
	for _, item := range result.Items {
		records = append(records, toPreferenceCandidateVO(item))
	}
	pages := 0
	if result.PageSize > 0 && result.Total > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}
	writeSuccess(c, pageResult[preferenceCandidateVO]{
		Records: records,
		Total:   result.Total,
		Size:    result.PageSize,
		Current: result.Page,
		Pages:   pages,
	})
}

func (h *Handler) ConfirmPreferenceCandidate(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	service, ok := h.requirePreferenceCandidateService(c)
	if !ok {
		return
	}
	candidate, err := service.ConfirmPreferenceCandidate(c.Request.Context(), longtermmemory.DecidePreferenceCandidateInput{
		UserID:      user.UserID,
		CandidateID: c.Param("candidateId"),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toPreferenceCandidateVO(candidate))
}

func (h *Handler) RejectPreferenceCandidate(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	service, ok := h.requirePreferenceCandidateService(c)
	if !ok {
		return
	}
	candidate, err := service.RejectPreferenceCandidate(c.Request.Context(), longtermmemory.DecidePreferenceCandidateInput{
		UserID:      user.UserID,
		CandidateID: c.Param("candidateId"),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toPreferenceCandidateVO(candidate))
}

func (h *Handler) requirePreferenceCandidateService(c *gin.Context) (longtermmemory.PreferenceCandidateService, bool) {
	if h == nil || h.preferenceCandidateService == nil {
		_ = c.Error(exception.NewServiceException("preference candidate service is required", nil))
		return nil, false
	}
	return h.preferenceCandidateService, true
}

func toPreferenceCandidateVO(candidate longtermmemory.PreferenceCandidate) preferenceCandidateVO {
	return preferenceCandidateVO{
		ID:               candidate.ID,
		ScopeType:        candidate.ScopeType,
		MemoryType:       candidate.MemoryType,
		CanonicalKey:     candidate.CanonicalKey,
		Summary:          candidate.Summary,
		Content:          candidate.Content,
		SourceMessageID:  candidate.SourceMessageID,
		ExtractionMethod: candidate.ExtractionMethod,
		Confidence:       candidate.Confidence,
		Status:           candidate.Status,
	}
}
