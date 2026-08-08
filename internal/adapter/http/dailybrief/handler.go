package dailybrief

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	dailybriefservice "local/rag-project/internal/app/dailybrief/service"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/middleware"
)

type Handler struct {
	readService                 *dailybriefservice.ReadService
	subscriptionService         *dailybriefservice.SubscriptionService
	topicCatalogService         *dailybriefservice.TopicCatalogService
	subscriptionSnapshotService dailybriefservice.SubscriptionSnapshotRefresher
}

func NewHandler(
	readService *dailybriefservice.ReadService,
	subscriptionService *dailybriefservice.SubscriptionService,
	topicCatalogService *dailybriefservice.TopicCatalogService,
	subscriptionSnapshotService dailybriefservice.SubscriptionSnapshotRefresher,
) *Handler {
	return &Handler{
		readService:                 readService,
		subscriptionService:         subscriptionService,
		topicCatalogService:         topicCatalogService,
		subscriptionSnapshotService: subscriptionSnapshotService,
	}
}

func (h *Handler) GetToday(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil || h.readService == nil {
		return
	}
	model, err := h.readService.GetToday(c.Request.Context(), user.UserID, time.Now())
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toIssueResponse(model))
}

func (h *Handler) GetIssueByDate(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil || h.readService == nil {
		return
	}
	date := strings.TrimSpace(c.Query("date"))
	if date == "" {
		_ = c.Error(exception.NewClientException("date query parameter is required", nil))
		return
	}
	model, err := h.readService.GetByDate(c.Request.Context(), user.UserID, date)
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toIssueResponse(model))
}

func (h *Handler) GetSubscription(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil || h.subscriptionService == nil {
		return
	}
	subscription, err := h.subscriptionService.GetByUserID(c.Request.Context(), user.UserID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toSubscriptionResponse(subscription))
}

func (h *Handler) UpdateSubscription(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil || h.subscriptionService == nil {
		return
	}
	var req updateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	subscription, err := h.subscriptionService.Upsert(c.Request.Context(), fromSubscriptionRequest(user.UserID, req))
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toSubscriptionResponse(subscription))
}

func (h *Handler) GetTopicCatalog(c *gin.Context) {
	if h.topicCatalogService == nil {
		_ = c.Error(exception.NewClientException("topic catalog is unavailable", nil))
		return
	}
	writeSuccess(c, topicCatalogResponse{Nodes: h.topicCatalogService.GetTree()})
}

func (h *Handler) RecomputeSubscriptionSnapshots(c *gin.Context) {
	if h.subscriptionSnapshotService == nil {
		_ = c.Error(exception.NewClientException("subscription snapshot service is unavailable", nil))
		return
	}
	var req recomputeSnapshotsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	result, err := h.subscriptionSnapshotService.Recompute(
		c.Request.Context(),
		dailybriefservice.SubscriptionSnapshotRecomputeInput{UserIDs: req.UserIDs},
	)
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, result)
}

func requireLoginUser(c *gin.Context) *contextx.LoginUser {
	user := contextx.Get(c)
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		_ = c.Error(exception.NewClientException("unauthorized", nil))
		return nil
	}
	return user
}

func writeSuccess[T any](c *gin.Context, data T) {
	c.JSON(http.StatusOK, convention.Result[T]{
		Code:      "0",
		RequestID: middleware.RequestID(c),
		Data:      data,
	})
}

func toIssueResponse(model dailybriefservice.IssueReadModel) issueResponse {
	response := issueResponse{
		PageState: model.PageState,
		BriefDate: model.BriefDate,
	}
	if model.LastGeneratedTime != nil {
		value := model.LastGeneratedTime.UTC().Format(time.RFC3339)
		response.LastGeneratedAt = &value
	}
	if model.PageState == dailybriefservice.PageStateReady {
		response.Issue = toIssueDTO(model)
	}
	return response
}
