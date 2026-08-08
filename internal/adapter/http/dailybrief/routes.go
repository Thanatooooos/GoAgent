package dailybrief

import (
	"github.com/gin-gonic/gin"

	dailybriefservice "local/rag-project/internal/app/dailybrief/service"
)

func RegisterRoutes(
	r gin.IRoutes,
	readService *dailybriefservice.ReadService,
	subscriptionService *dailybriefservice.SubscriptionService,
	topicCatalogService *dailybriefservice.TopicCatalogService,
) {
	handler := NewHandler(readService, subscriptionService, topicCatalogService, nil)
	r.GET("/daily-brief/topic-catalog", handler.GetTopicCatalog)
	r.GET("/daily-brief/today", handler.GetToday)
	r.GET("/daily-brief/issues", handler.GetIssueByDate)
	r.GET("/daily-brief/subscription", handler.GetSubscription)
	r.PUT("/daily-brief/subscription", handler.UpdateSubscription)
}

func RegisterAdminRoutes(r gin.IRoutes, snapshotService dailybriefservice.SubscriptionSnapshotRefresher) {
	handler := NewHandler(nil, nil, nil, snapshotService)
	r.POST("/daily-brief/subscription-snapshots/recompute", handler.RecomputeSubscriptionSnapshots)
}
