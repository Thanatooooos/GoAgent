package scheduledtask

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	"local/rag-project/internal/app/scheduledtask/domain"
	scheduledservice "local/rag-project/internal/app/scheduledtask/service"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/middleware"
)

type Handler struct {
	Store    *storepkg.Store
	Proposer scheduledservice.Proposer
}

type taskDetail struct {
	Task    domain.Task    `json:"task"`
	Version domain.Version `json:"version"`
}

type draftRequest struct {
	TaskID               string                  `json:"taskId"`
	BaseVersion          int                     `json:"baseVersion"`
	OriginConversationID string                  `json:"originConversationId"`
	Config               storepkg.ProposedConfig `json:"config"`
}

func RegisterRoutes(r gin.IRoutes, store *storepkg.Store, proposer scheduledservice.Proposer) {
	h := &Handler{Store: store, Proposer: proposer}
	r.GET("/scheduled-tasks", h.list)
	r.GET("/scheduled-tasks/unread", h.unread)
	r.POST("/scheduled-tasks/conversations/:conversationId/read", h.markRead)
	r.GET("/scheduled-tasks/conversations/:conversationId/drafts", h.listDrafts)
	r.POST("/scheduled-tasks/drafts", h.createDraft)
	r.POST("/scheduled-tasks/drafts/:draftId/confirm", h.confirmDraft)
	r.GET("/scheduled-tasks/:taskId", h.detail)
	r.GET("/scheduled-tasks/:taskId/runs", h.runs)
	r.GET("/scheduled-tasks/:taskId/latest-report", h.latestReport)
	r.GET("/scheduled-tasks/:taskId/runs/:occurrenceId", h.runDetail)
	r.POST("/scheduled-tasks/:taskId/pause", h.pause)
	r.POST("/scheduled-tasks/:taskId/resume", h.resume)
	r.DELETE("/scheduled-tasks/:taskId", h.deleteTask)
}

func userID(c *gin.Context) string {
	user := contextx.Get(c)
	if user == nil {
		return ""
	}
	return strings.TrimSpace(user.UserID)
}

func success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, convention.Result[any]{Code: "0", RequestID: middleware.RequestID(c), Data: data})
}

func fail(c *gin.Context, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status = http.StatusNotFound
	}
	c.JSON(status, convention.Result[any]{Code: "1", Message: err.Error(), RequestID: middleware.RequestID(c)})
}

func (h *Handler) list(c *gin.Context) {
	tasks, err := h.Store.List(c.Request.Context(), userID(c))
	if err != nil {
		fail(c, err)
		return
	}
	items := make([]taskDetail, 0, len(tasks))
	for _, task := range tasks {
		_, version, err := h.Store.Get(c.Request.Context(), userID(c), task.ID)
		if err != nil {
			fail(c, err)
			return
		}
		items = append(items, taskDetail{Task: task, Version: version})
	}
	success(c, items)
}

func (h *Handler) detail(c *gin.Context) {
	task, version, err := h.Store.Get(c.Request.Context(), userID(c), c.Param("taskId"))
	if err != nil {
		fail(c, err)
		return
	}
	success(c, taskDetail{Task: task, Version: version})
}

func (h *Handler) runs(c *gin.Context) {
	if _, _, err := h.Store.Get(c.Request.Context(), userID(c), c.Param("taskId")); err != nil {
		fail(c, err)
		return
	}
	runs, err := h.Store.ListRuns(c.Request.Context(), userID(c), c.Param("taskId"))
	if err != nil {
		fail(c, err)
		return
	}
	success(c, runs)
}

func (h *Handler) latestReport(c *gin.Context) {
	report, err := h.Store.LatestReport(c.Request.Context(), userID(c), c.Param("taskId"))
	if err != nil {
		fail(c, err)
		return
	}
	success(c, report)
}

func (h *Handler) createDraft(c *gin.Context) {
	var req draftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	// Review the prompt's implicit schedule and reporting semantics before
	// saving the complete preview. This does not update effective configuration.
	if req.TaskID != "" {
		task, _, err := h.Store.Get(c.Request.Context(), userID(c), req.TaskID)
		if err != nil {
			fail(c, err)
			return
		}
		if task.CurrentVersion != req.BaseVersion {
			fail(c, errors.New("task version conflict"))
			return
		}
	}
	reviewed, err := h.Proposer.ReviewConfig(c.Request.Context(), domain.Version{
		Name: req.Config.Name, Prompt: req.Config.Prompt, Schedule: req.Config.Schedule, ReportMode: req.Config.ReportMode,
		ConditionKind: req.Config.ConditionKind, KnowledgeBaseIDs: req.Config.KnowledgeBaseIDs,
		AllowedWebDomains: req.Config.AllowedWebDomains, AllowedToolIDs: req.Config.AllowedToolIDs,
	}, time.Now())
	if err != nil {
		fail(c, err)
		return
	}
	req.Config = storepkg.ProposedConfig{Name: reviewed.Name, Prompt: reviewed.Prompt, Schedule: reviewed.Schedule,
		ReportMode: reviewed.ReportMode, ConditionKind: reviewed.ConditionKind, KnowledgeBaseIDs: reviewed.KnowledgeBaseIDs,
		AllowedWebDomains: reviewed.AllowedWebDomains, AllowedToolIDs: reviewed.AllowedToolIDs}
	draft, err := h.Store.CreateDraft(c.Request.Context(), userID(c), req.OriginConversationID,
		req.TaskID, req.BaseVersion, req.Config, time.Now())
	if err != nil {
		fail(c, err)
		return
	}
	success(c, draft)
}

// listDrafts restores the confirmation cards of a conversation after a reload,
// which drops the tool events that carried them.
func (h *Handler) listDrafts(c *gin.Context) {
	drafts, err := h.Store.ListPendingDrafts(c.Request.Context(), userID(c), c.Param("conversationId"), time.Now())
	if err != nil {
		fail(c, err)
		return
	}
	success(c, drafts)
}

func (h *Handler) runDetail(c *gin.Context) {
	detail, err := h.Store.GetRun(c.Request.Context(), userID(c), c.Param("taskId"), c.Param("occurrenceId"))
	if err != nil {
		fail(c, err)
		return
	}
	success(c, detail)
}

func (h *Handler) confirmDraft(c *gin.Context) {
	var req struct {
		AllowDuplicate bool `json:"allowDuplicate"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, err)
			return
		}
	}
	task, version, err := h.Store.ConfirmDraft(c.Request.Context(), userID(c), c.Param("draftId"), time.Now(), req.AllowDuplicate)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, taskDetail{Task: task, Version: version})
}

func (h *Handler) pause(c *gin.Context) {
	if err := h.Store.SetStatus(c.Request.Context(), userID(c), c.Param("taskId"), domain.TaskPaused, time.Time{}); err != nil {
		fail(c, err)
		return
	}
	success(c, nil)
}

func (h *Handler) resume(c *gin.Context) {
	if err := h.Store.Resume(c.Request.Context(), userID(c), c.Param("taskId"), time.Now()); err != nil {
		fail(c, err)
		return
	}
	success(c, nil)
}

func (h *Handler) deleteTask(c *gin.Context) {
	if err := h.Store.Delete(c.Request.Context(), userID(c), c.Param("taskId")); err != nil {
		fail(c, err)
		return
	}
	success(c, nil)
}

func (h *Handler) unread(c *gin.Context) {
	items, err := h.Store.ListUnread(c.Request.Context(), userID(c))
	if err != nil {
		fail(c, err)
		return
	}
	success(c, items)
}

func (h *Handler) markRead(c *gin.Context) {
	if err := h.Store.MarkRead(c.Request.Context(), userID(c), c.Param("conversationId")); err != nil {
		fail(c, err)
		return
	}
	success(c, nil)
}
