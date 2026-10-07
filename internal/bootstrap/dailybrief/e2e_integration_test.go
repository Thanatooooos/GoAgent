package dailybrief

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	dailybriefhttp "local/rag-project/internal/adapter/http/dailybrief"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresdailybrief "local/rag-project/internal/adapter/repository/postgres/dailybrief"
	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	dailybriefschedule "local/rag-project/internal/app/dailybrief/schedule"
	dailybriefservice "local/rag-project/internal/app/dailybrief/service"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/middleware"

	"gorm.io/gorm"
)

const e2eArtifactJSON = `{
  "headline": "Daily AI Brief E2E",
  "topSummary": "Integration test brief across models and developer trends.",
  "sections": [
    {
      "key": "tech.ai.models",
      "title": "AI Models",
      "items": [
        {
          "title": "Model Safety Eval Updates",
          "summary": "New safety evaluation routines for production model releases.",
          "whyItMatters": "Production teams need repeatable safety gates before launch.",
          "url": "https://openai.com/blog/model-safety-eval-updates",
          "source": "openai-blog",
          "topic": "tech.ai.models"
        }
      ]
    },
    {
      "key": "tech.dev",
      "title": "Developer Trends",
      "items": [
        {
          "title": "Efficient RLHF for Small Teams",
          "summary": "How a two-engineer team shipped a practical RLHF loop.",
          "whyItMatters": "Small teams can still adopt RLHF with lean workflows.",
          "url": "https://example.com/rlhf-small-teams",
          "source": "hacker-news",
          "topic": "tech.dev"
        }
      ]
    }
  ]
}`

func TestDailyBriefPipelineE2E(t *testing.T) {
	if os.Getenv("RAG_INTEGRATION_DAILY_BRIEF") != "1" {
		t.Skip("set RAG_INTEGRATION_DAILY_BRIEF=1 to run daily brief end-to-end integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	clock := &frozenClock{t: time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)}
	env := newDailyBriefE2EEnv(t, &staticTaskRuntimeStub{response: e2eArtifactJSON}, clock)

	userID := mustE2EUserID(t)
	subscription := domain.NewSubscription(
		userID,
		"UTC",
		"08:00",
		[]string{domain.TopicKeyTechAIModels, domain.TopicKeyTechDev},
		[]string{domain.SourceKeyHackerNews, domain.SourceKeyOpenAIBlog},
	)
	subscription.Enabled = true

	saved, err := env.subscriptionService.Upsert(ctx, subscription)
	if err != nil {
		t.Fatalf("upsert subscription: %v", err)
	}
	if len(saved.Sources) == 0 {
		t.Fatalf("expected derived sources snapshot, got %+v", saved)
	}

	if err := env.processor.ProcessSubscription(ctx, saved, domain.GenerationRunTriggerTypeScheduled); err != nil {
		t.Fatalf("process subscription: %v", err)
	}

	model, err := env.readService.GetToday(ctx, userID, clock.Now())
	if err != nil {
		t.Fatalf("get today: %v", err)
	}
	if model.PageState != dailybriefservice.PageStateReady {
		t.Fatalf("expected ready page state, got %q issue=%+v", model.PageState, model.Issue)
	}
	if model.Issue.Headline != "Daily AI Brief E2E" {
		t.Fatalf("unexpected headline: %q", model.Issue.Headline)
	}
	if len(model.Items) != 2 {
		t.Fatalf("expected 2 published items, got %d", len(model.Items))
	}

	router := newDailyBriefE2ERouter(env.readService, env.subscriptionService)
	req := httptest.NewRequest(http.MethodGet, "/daily-brief/issues?date=2026-06-29", nil)
	req.Header.Set("X-User-ID", userID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET issue status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data struct {
			PageState string `json:"pageState"`
			Issue     struct {
				Headline string `json:"headline"`
				Items    []struct {
					Source string `json:"source"`
				} `json:"items"`
			} `json:"issue"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if payload.Data.PageState != "ready" {
		t.Fatalf("expected ready via http, got %q", payload.Data.PageState)
	}
	if payload.Data.Issue.Headline != "Daily AI Brief E2E" {
		t.Fatalf("unexpected http headline: %q", payload.Data.Issue.Headline)
	}
	if len(payload.Data.Issue.Items) != 2 {
		t.Fatalf("expected 2 http items, got %d", len(payload.Data.Issue.Items))
	}

	putBody := bytes.NewBufferString(`{"enabled":true,"timezone":"UTC","deliveryTimeLocal":"08:00","topics":["tech.ai.models","tech.dev"],"sources":["hacker-news","openai-blog"]}`)
	putReq := httptest.NewRequest(http.MethodPut, "/daily-brief/subscription", putBody)
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("X-User-ID", userID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT subscription status=%d body=%s", putRec.Code, putRec.Body.String())
	}

	snapshot := env.metrics.Snapshot()
	if snapshot.SuccessfulRuns != 1 || snapshot.FinalItemCount != 2 {
		t.Fatalf("unexpected metrics snapshot: %+v", snapshot)
	}
}

func TestDailyBriefRetryE2E(t *testing.T) {
	if os.Getenv("RAG_INTEGRATION_DAILY_BRIEF") != "1" {
		t.Skip("set RAG_INTEGRATION_DAILY_BRIEF=1 to run daily brief end-to-end integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	clock := &frozenClock{t: time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)}
	taskRuntime := &flipTaskRuntimeStub{artifact: e2eArtifactJSON}
	env := newDailyBriefE2EEnv(t, taskRuntime, clock)

	userID := mustE2EUserID(t)
	subscription := domain.NewSubscription(
		userID,
		"UTC",
		"08:00",
		[]string{domain.TopicKeyTechAIModels, domain.TopicKeyTechDev},
		[]string{domain.SourceKeyHackerNews, domain.SourceKeyOpenAIBlog},
	)
	subscription.Enabled = true
	saved, err := env.subscriptionService.Upsert(ctx, subscription)
	if err != nil {
		t.Fatalf("upsert subscription: %v", err)
	}

	if err := env.processor.ProcessSubscription(ctx, saved, domain.GenerationRunTriggerTypeScheduled); err != nil {
		t.Fatalf("first scheduled run returned unexpected error: %v", err)
	}

	failedModel, err := env.readService.GetToday(ctx, userID, clock.Now())
	if err != nil {
		t.Fatalf("get today after failure: %v", err)
	}
	if failedModel.PageState != dailybriefservice.PageStateFailed {
		t.Fatalf("expected failed page state, got %q", failedModel.PageState)
	}

	clock.Set(clock.Now().Add(31 * time.Minute))
	runs, err := env.generationRunRepo.ListRetryEligible(ctx, port.GenerationRunRetryEligibleFilter{
		FailedBefore: clock.Now(),
		Limit:        20,
	})
	if err != nil {
		t.Fatalf("list retry eligible runs: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected at least one retry-eligible failed run")
	}

	var target domain.GenerationRun
	for _, run := range runs {
		if run.UserID == userID {
			target = run
			break
		}
	}
	if target.ID == "" {
		t.Fatalf("expected failed run for user %q, got %#v", userID, runs)
	}

	if err := env.processor.ProcessRetry(ctx, target); err != nil {
		t.Fatalf("process retry: %v", err)
	}

	readyModel, err := env.readService.GetToday(ctx, userID, clock.Now())
	if err != nil {
		t.Fatalf("get today after retry: %v", err)
	}
	if readyModel.PageState != dailybriefservice.PageStateReady {
		t.Fatalf("expected ready page state after retry, got %q", readyModel.PageState)
	}
	if taskRuntime.Calls() < 2 {
		t.Fatalf("expected task runtime to be called at least twice, got %d", taskRuntime.Calls())
	}
}

type dailyBriefE2EEnv struct {
	db                  *gorm.DB
	subscriptionService *dailybriefservice.SubscriptionService
	readService         *dailybriefservice.ReadService
	processor           *dailybriefschedule.Processor
	generationRunRepo   *postgresdailybrief.GenerationRunRepository
	metrics             *dailybriefservice.MetricsService
}

func newDailyBriefE2EEnv(t *testing.T, taskRuntime conversationruntime.TaskRuntime, clock *frozenClock) *dailyBriefE2EEnv {
	t.Helper()

	db, err := postgresrepo.NewGormDB(config.DataSourceConfig{
		Url:      getenvDefault("POSTGRES_URL", "jdbc:postgresql://localhost:5432/ragent"),
		Username: getenvDefault("POSTGRES_USER", "postgres"),
		Password: getenvDefault("POSTGRES_PASSWORD", "postgres"),
	})
	if err != nil {
		t.Fatalf("new postgres db: %v", err)
	}
	if err := postgresrepo.RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	subscriptionRepo := postgresdailybrief.NewSubscriptionRepository(db)
	issueRepo := postgresdailybrief.NewIssueRepository(db)
	itemRepo := postgresdailybrief.NewItemRepository(db)
	generationRunRepo := postgresdailybrief.NewGenerationRunRepository(db)
	publishTx := postgresdailybrief.NewPublishTransaction(db)

	registry, err := dailybriefservice.NewDefaultSourceRegistry()
	if err != nil {
		t.Fatalf("create source registry: %v", err)
	}
	hnSpec, _ := domain.SourceFeedSpecByKey(domain.SourceKeyHackerNews)
	openAISpec, _ := domain.SourceFeedSpecByKey(domain.SourceKeyOpenAIBlog)
	fixtureClient := &fixtureHTTPClient{
		bodies: map[string][]byte{
			hnSpec.URL:     loadE2EFixture(t, "hacker-news.rss.xml"),
			openAISpec.URL: loadE2EFixture(t, "openai-blog.rss.xml"),
		},
	}

	generationCfg := config.DailyBriefGenerationConfig{
		MaxCandidates: 20,
		MaxItems:      5,
		PromptVersion: "e2e-v1",
		Model:         "e2e-model",
	}
	metrics := dailybriefservice.NewMetricsService()
	collector := dailybriefservice.NewSourceCollector(registry, fixtureClient)
	pipeline := dailybriefservice.NewCandidatePipeline(generationCfg)
	generator := dailybriefservice.NewRuntimeBriefGenerator(taskRuntime, generationCfg)
	publisher := dailybriefservice.NewPublisher(publishTx)
	orchestrator := dailybriefservice.NewGenerationOrchestrator(
		collector,
		pipeline,
		generator,
		publisher,
		dailybriefservice.NewIssueService(issueRepo),
		dailybriefservice.NewGenerationRunService(generationRunRepo),
		generationCfg,
		metrics,
	)
	processor := dailybriefschedule.NewProcessor(
		orchestrator,
		subscriptionRepo,
		issueRepo,
		generationRunRepo,
		dailybriefschedule.NewRetryPolicy(2, 30),
		metrics,
		clock.Now,
	)

	return &dailyBriefE2EEnv{
		db:                  db,
		subscriptionService: dailybriefservice.NewSubscriptionService(subscriptionRepo),
		readService:         dailybriefservice.NewReadService(subscriptionRepo, issueRepo, itemRepo),
		processor:           processor,
		generationRunRepo:   generationRunRepo,
		metrics:             metrics,
	}
}

type frozenClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *frozenClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *frozenClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

type fixtureHTTPClient struct {
	bodies map[string][]byte
}

func (c *fixtureHTTPClient) Get(_ context.Context, url string) ([]byte, error) {
	body, ok := c.bodies[url]
	if !ok {
		return nil, fmt.Errorf("unexpected url %q", url)
	}
	return body, nil
}

type staticTaskRuntimeStub struct {
	response string
}

func (s *staticTaskRuntimeStub) RunTask(context.Context, conversationruntime.TaskRequest) (conversationruntime.RunResult, error) {
	return conversationruntime.RunResult{Status: conversationruntime.StatusCompleted, AssistantContent: s.response}, nil
}

type flipTaskRuntimeStub struct {
	mu       sync.Mutex
	calls    int
	artifact string
}

func (s *flipTaskRuntimeStub) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *flipTaskRuntimeStub) RunTask(context.Context, conversationruntime.TaskRequest) (conversationruntime.RunResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls == 1 {
		return conversationruntime.RunResult{}, fmt.Errorf("simulated generation failure")
	}
	return conversationruntime.RunResult{Status: conversationruntime.StatusCompleted, AssistantContent: s.artifact}, nil
}

func mustE2EUserID(t *testing.T) string {
	t.Helper()
	id, err := distributedid.NextID()
	if err != nil {
		t.Fatalf("next user id: %v", err)
	}
	return fmt.Sprintf("%d", id)
}

func loadE2EFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "dailybrief", "sources", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", path, err)
	}
	return body
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func newDailyBriefE2ERouter(
	readService *dailybriefservice.ReadService,
	subscriptionService *dailybriefservice.SubscriptionService,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware(), middleware.ErrorHandlerMiddleware(), func(c *gin.Context) {
		userID := strings.TrimSpace(c.GetHeader("X-User-ID"))
		if userID == "" {
			userID = "alice"
		}
		contextx.Set(c, &contextx.LoginUser{UserID: userID})
		c.Next()
	})
	dailybriefhttp.RegisterRoutes(router, readService, subscriptionService, dailybriefservice.NewTopicCatalogService())
	return router
}
