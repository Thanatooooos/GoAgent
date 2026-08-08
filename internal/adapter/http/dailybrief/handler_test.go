package dailybrief_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	dailybriefhttp "local/rag-project/internal/adapter/http/dailybrief"
	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	dailybriefservice "local/rag-project/internal/app/dailybrief/service"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/middleware"
)

func newDailyBriefRouter(
	readService *dailybriefservice.ReadService,
	subscriptionService *dailybriefservice.SubscriptionService,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware(), middleware.ErrorHandlerMiddleware(), testDailyBriefUserMiddleware())
	dailybriefhttp.RegisterRoutes(router, readService, subscriptionService, dailybriefservice.NewTopicCatalogService())
	return router
}

func newDailyBriefAdminRouter(refresh *stubSnapshotRefreshService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware(), middleware.ErrorHandlerMiddleware(), testDailyBriefUserMiddleware())
	dailybriefhttp.RegisterAdminRoutes(router, refresh)
	return router
}

func testDailyBriefUserMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := strings.TrimSpace(c.GetHeader("X-User-ID"))
		if userID == "" {
			userID = "alice"
		}
		contextx.Set(c, &contextx.LoginUser{UserID: userID})
		c.Next()
	}
}

func TestHandlerGetTopicCatalogReturnsRecursiveNodes(t *testing.T) {
	router := newDailyBriefRouter(
		dailybriefservice.NewReadService(&handlerSubscriptionRepo{}, &handlerIssueRepo{}, &handlerItemRepo{}),
		dailybriefservice.NewSubscriptionService(&handlerSubscriptionRepo{}),
	)

	req := httptest.NewRequest(http.MethodGet, "/daily-brief/topic-catalog", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"key\":\"tech.ai.models\"") {
		t.Fatalf("expected topic leaf in payload, got %s", rec.Body.String())
	}
}

func TestHandlerGetIssueByDateReturnsReadyIssue(t *testing.T) {
	generatedAt := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)
	readService := dailybriefservice.NewReadService(
		&handlerSubscriptionRepo{subscription: domain.NewSubscription("alice", "UTC", "08:00", []string{domain.TopicKeyTechAIModels}, []string{"hacker-news"})},
		&handlerIssueRepo{issue: domain.Issue{
			ID:          "issue-1",
			UserID:      "alice",
			BriefDate:   "2026-06-29",
			Status:      domain.IssueStatusReady,
			Headline:    "AI Headline",
			TopSummary:  "Top summary",
			GeneratedAt: &generatedAt,
		}},
		&handlerItemRepo{items: []domain.Item{{
			ID:           "item-1",
			IssueID:      "issue-1",
			SectionKey:   "tech.ai.models",
			Title:        "Story",
			Summary:      "Summary",
			WhyItMatters: "Because",
			URL:          "https://example.com",
			Source:       "hacker-news",
			Topic:        "tech.ai.models",
		}}},
	)
	router := newDailyBriefRouter(readService, dailybriefservice.NewSubscriptionService(&handlerSubscriptionRepo{}))

	req := httptest.NewRequest(http.MethodGet, "/daily-brief/issues?date=2026-06-29", nil)
	req.Header.Set("X-User-ID", "alice")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Code string `json:"code"`
		Data struct {
			PageState string `json:"pageState"`
			Issue     struct {
				Headline string `json:"headline"`
				Items    []struct {
					WhyItMatters string `json:"whyItMatters"`
				} `json:"items"`
			} `json:"issue"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if payload.Data.PageState != "ready" {
		t.Fatalf("expected ready page state, got %q", payload.Data.PageState)
	}
	if payload.Data.Issue.Headline != "AI Headline" {
		t.Fatalf("unexpected headline: %q", payload.Data.Issue.Headline)
	}
	if len(payload.Data.Issue.Items) != 1 || payload.Data.Issue.Items[0].WhyItMatters != "Because" {
		t.Fatalf("unexpected items: %+v", payload.Data.Issue.Items)
	}
}

func TestHandlerUpdateSubscriptionIgnoresClientSources(t *testing.T) {
	repo := &handlerSubscriptionRepo{}
	router := newDailyBriefRouter(
		dailybriefservice.NewReadService(repo, &handlerIssueRepo{}, &handlerItemRepo{}),
		dailybriefservice.NewSubscriptionService(repo),
	)

	body := bytes.NewBufferString(`{"enabled":true,"timezone":"UTC","deliveryTimeLocal":"08:00","topics":["tech.ai.models"],"sources":["malicious"]}`)
	req := httptest.NewRequest(http.MethodPut, "/daily-brief/subscription", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "alice")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if slices.Contains(repo.lastUpsert.Sources, "malicious") {
		t.Fatalf("expected derived sources, got %+v", repo.lastUpsert.Sources)
	}
}

func TestHandlerUpdateSubscriptionPersistsUserID(t *testing.T) {
	repo := &handlerSubscriptionRepo{}
	subscriptionService := dailybriefservice.NewSubscriptionService(repo)
	router := newDailyBriefRouter(
		dailybriefservice.NewReadService(repo, &handlerIssueRepo{}, &handlerItemRepo{}),
		subscriptionService,
	)

	body := bytes.NewBufferString(`{"enabled":true,"timezone":"UTC","deliveryTimeLocal":"08:00","topics":["tech.ai.models"]}`)
	req := httptest.NewRequest(http.MethodPut, "/daily-brief/subscription", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "alice")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastUpsert.UserID != "alice" {
		t.Fatalf("expected upsert user id alice, got %q", repo.lastUpsert.UserID)
	}
}

func TestAdminRecomputeRouteTriggersSnapshotRefresh(t *testing.T) {
	refresh := &stubSnapshotRefreshService{}
	router := newDailyBriefAdminRouter(refresh)

	req := httptest.NewRequest(http.MethodPost, "/daily-brief/subscription-snapshots/recompute", bytes.NewBufferString(`{"userIds":["alice"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !reflect.DeepEqual(refresh.lastInput.UserIDs, []string{"alice"}) {
		t.Fatalf("unexpected recompute input: %+v", refresh.lastInput)
	}
}

type stubSnapshotRefreshService struct {
	lastInput dailybriefservice.SubscriptionSnapshotRecomputeInput
}

func (s *stubSnapshotRefreshService) Recompute(ctx context.Context, input dailybriefservice.SubscriptionSnapshotRecomputeInput) (dailybriefservice.SubscriptionSnapshotRecomputeResult, error) {
	s.lastInput = input
	return dailybriefservice.SubscriptionSnapshotRecomputeResult{ScannedCount: 1, UpdatedCount: 1}, nil
}

type handlerSubscriptionRepo struct {
	subscription domain.Subscription
	lastUpsert   domain.Subscription
}

func (s *handlerSubscriptionRepo) Upsert(ctx context.Context, subscription domain.Subscription) (domain.Subscription, error) {
	s.lastUpsert = subscription
	s.subscription = subscription
	return subscription, nil
}

func (s *handlerSubscriptionRepo) GetByUserID(ctx context.Context, userID string) (domain.Subscription, error) {
	if s.subscription.UserID == userID {
		return s.subscription, nil
	}
	return domain.Subscription{}, nil
}

func (s *handlerSubscriptionRepo) List(ctx context.Context, filter port.SubscriptionListFilter) ([]domain.Subscription, error) {
	return nil, nil
}

func (s *handlerSubscriptionRepo) TryAcquireLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	return true, nil
}

func (s *handlerSubscriptionRepo) RenewLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time) (bool, error) {
	return true, nil
}

func (s *handlerSubscriptionRepo) ReleaseLock(ctx context.Context, lease domain.SubscriptionLockLease) (bool, error) {
	return true, nil
}

type handlerIssueRepo struct {
	issue domain.Issue
}

func (s *handlerIssueRepo) Create(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	return issue, nil
}

func (s *handlerIssueRepo) Update(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	return issue, nil
}

func (s *handlerIssueRepo) GetByID(ctx context.Context, id string) (domain.Issue, error) {
	return s.issue, nil
}

func (s *handlerIssueRepo) GetByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.Issue, error) {
	if s.issue.UserID == userID && s.issue.BriefDate == briefDate {
		return s.issue, nil
	}
	return domain.Issue{}, nil
}

func (s *handlerIssueRepo) List(ctx context.Context, filter port.IssueListFilter) ([]domain.Issue, error) {
	return nil, nil
}

type handlerItemRepo struct {
	items []domain.Item
}

func (s *handlerItemRepo) Create(ctx context.Context, item domain.Item) (domain.Item, error) {
	return item, nil
}

func (s *handlerItemRepo) ReplaceIssueItems(ctx context.Context, issueID string, items []domain.Item) error {
	return nil
}

func (s *handlerItemRepo) List(ctx context.Context, filter port.ItemListFilter) ([]domain.Item, error) {
	return append([]domain.Item(nil), s.items...), nil
}
