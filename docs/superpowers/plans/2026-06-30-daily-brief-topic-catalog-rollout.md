# Daily Brief Topic Catalog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a safe topic-first Daily Brief subscription flow where users save leaf topics, the backend derives and persists `sources` as an execution snapshot, and the frontend consumes a backend-driven topic catalog.

**Architecture:** Keep the current generation runtime stable by preserving persisted `sources[]` as the collection input, but demote `sources[]` from user-owned form data to backend-derived state. Introduce a tree-based topic catalog and source-expansion helpers in the domain layer, expose a topic-catalog API plus an admin-only snapshot recompute path, and migrate the frontend from hard-coded topic/source checkboxes to catalog-driven rendering.

**Tech Stack:** Go, Gin, GORM/PostgreSQL repositories, React 18, TypeScript, Vite

---

## File Structure

**Backend domain and services**

- Modify: `internal/app/dailybrief/domain/topic_catalog.go`
  - Replace flat topic constants with a generalized topic tree and helper lookups.
- Modify: `internal/app/dailybrief/domain/source_feed_catalog.go`
  - Rebind source feeds to hierarchical leaf topics and add topic-to-source expansion helpers.
- Modify: `internal/app/dailybrief/domain/constants_test.go`
  - Update current catalog assertions to the new keys.
- Create: `internal/app/dailybrief/domain/topic_catalog_test.go`
  - Add focused tests for tree traversal, breadcrumbs, and selectable leaf validation.
- Create: `internal/app/dailybrief/domain/source_feed_catalog_test.go`
  - Add tests for topic-to-source expansion and `hasSources` behavior.
- Modify: `internal/app/dailybrief/service/subscription_service.go`
  - Derive `sources[]` from `topics[]` during save and tighten validation around selectable leaf topics.
- Create: `internal/app/dailybrief/service/subscription_snapshot_service.go`
  - Add an explicit snapshot recompute service for admin/operator use.
- Create: `internal/app/dailybrief/service/subscription_snapshot_service_test.go`
  - Add recompute-path tests.
- Modify: `internal/app/dailybrief/service/subscription_service_test.go`
  - Update expectations so request `sources[]` are ignored and derived from topics.
- Modify: `internal/app/dailybrief/service/candidate_pipeline.go`
  - Update source-to-topic inference to the new leaf topic keys.
- Modify: `internal/app/dailybrief/service/prompt_builder.go`
  - Use new topic display helpers and hierarchical labels in prompts.
- Modify: `internal/app/dailybrief/service/prompt_builder_test.go`
  - Update prompt expectations for the new hierarchical labels.

**HTTP and bootstrap**

- Create: `internal/app/dailybrief/service/topic_catalog_service.go`
  - Provide a read model for the topic-catalog API.
- Create: `internal/app/dailybrief/service/topic_catalog_service_test.go`
  - Verify the API-facing tree shape and `hasSources` projection.
- Modify: `internal/adapter/http/dailybrief/dto.go`
  - Remove editable `sources` from subscription DTOs and add topic-catalog DTOs.
- Modify: `internal/adapter/http/dailybrief/handler.go`
  - Serve `GET /daily-brief/topic-catalog`, update subscription PUT handling, and expose admin recompute.
- Modify: `internal/adapter/http/dailybrief/routes.go`
  - Split user routes and admin-only routes.
- Modify: `internal/adapter/http/dailybrief/handler_test.go`
  - Add HTTP tests for topic catalog, subscription save behavior, and admin recompute.
- Modify: `internal/bootstrap/dailybrief/runtime.go`
  - Wire the new topic catalog and snapshot services.
- Modify: `cmd/server/main.go`
  - Register admin-only Daily Brief routes under the existing admin group.

**Frontend**

- Modify: `frontend/src/types/dailyBrief.ts`
  - Replace hard-coded topic/source option lists with catalog types and topic display helpers.
- Modify: `frontend/src/services/dailyBriefService.ts`
  - Fetch topic catalog and stop sending `sources` in subscription updates.
- Create: `frontend/src/lib/dailyBriefTopicCatalog.ts`
  - Add frontend helpers for flattening the tree, disabled-leaf detection, and breadcrumb labels.
- Modify: `frontend/src/pages/DailyBriefPage.tsx`
  - Render subscription UI from the backend catalog and remove source checkboxes.

**Integration and verification**

- Modify: `internal/bootstrap/dailybrief/e2e_integration_test.go`
  - Update request bodies and assertions to the new subscription contract.

---

### Task 1: Introduce a Tree-Based Topic Catalog and Source Expansion Helpers

**Files:**
- Create: `internal/app/dailybrief/domain/topic_catalog_test.go`
- Create: `internal/app/dailybrief/domain/source_feed_catalog_test.go`
- Modify: `internal/app/dailybrief/domain/topic_catalog.go`
- Modify: `internal/app/dailybrief/domain/source_feed_catalog.go`
- Modify: `internal/app/dailybrief/domain/constants_test.go`

- [ ] **Step 1: Write the failing domain tests**

```go
package domain_test

import (
	"testing"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestTopicCatalogTreeExposesSelectableLeafTopics(t *testing.T) {
	tree := domain.TopicCatalogTree()
	if len(tree) == 0 {
		t.Fatal("expected non-empty topic catalog tree")
	}
	if !domain.IsTopicKeyKnown("tech.ai.models") {
		t.Fatal("expected tech.ai.models to be known")
	}
	if !domain.IsLeafTopicKey("tech.ai.models") {
		t.Fatal("expected tech.ai.models to be a leaf")
	}
	if !domain.IsTopicKeySelectable("tech.ai.models") {
		t.Fatal("expected tech.ai.models to be selectable")
	}
	if domain.IsTopicKeySelectable("tech") {
		t.Fatal("expected non-leaf tech to be non-selectable")
	}
}

func TestTopicBreadcrumbBuildsHumanReadableLabel(t *testing.T) {
	got := domain.TopicBreadcrumb("tech.ai.models")
	want := "科技 · AI · 模型发布"
	if got != want {
		t.Fatalf("unexpected breadcrumb: got %q want %q", got, want)
	}
}

func TestSourceKeysForTopicsExpandsAndSortsUniqueSources(t *testing.T) {
	got := domain.SourceKeysForTopics([]string{"tech.ai.models", "tech.ai.research"})
	want := []string{
		"anthropic-blog",
		"arxiv-cs-ai",
		"arxiv-cs-cl",
		"arxiv-cs-lg",
		"google-deepmind-blog",
		"meta-ai-blog",
		"openai-blog",
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected source count: got %d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected source at %d: got %q want %q (%v)", i, got[i], want[i], got)
		}
	}
	if !domain.TopicHasSources("tech.ai.models") {
		t.Fatal("expected tech.ai.models to report sources")
	}
	if domain.TopicHasSources("art.design") {
		t.Fatal("expected art.design to report no sources during phase 1")
	}
}
```

- [ ] **Step 2: Run the domain tests to verify they fail**

Run:

```bash
go test ./internal/app/dailybrief/domain -run "TestTopicCatalogTreeExposesSelectableLeafTopics|TestTopicBreadcrumbBuildsHumanReadableLabel|TestSourceKeysForTopicsExpandsAndSortsUniqueSources" -count=1
```

Expected: FAIL because `TopicCatalogTree`, `IsTopicKeyKnown`, `IsLeafTopicKey`, `IsTopicKeySelectable`, `TopicBreadcrumb`, `SourceKeysForTopics`, and `TopicHasSources` do not exist yet.

- [ ] **Step 3: Implement the tree-based catalog and source expansion helpers**

```go
package domain

import (
	"sort"
	"strings"
)

type TopicNode struct {
	Key         string
	ParentKey   string
	DisplayName string
	Description string
	SortOrder   int
	Selectable  bool
	Enabled     bool
	Children    []TopicNode
}

const (
	TopicKeyTech           = "tech"
	TopicKeyTechAI         = "tech.ai"
	TopicKeyTechAIModels   = "tech.ai.models"
	TopicKeyTechAIResearch = "tech.ai.research"
	TopicKeyTechAITools    = "tech.ai.tools"
	TopicKeyTechDev        = "tech.dev"
	TopicKeyTechStartups   = "tech.startups"
)

var topicCatalogFlat = []TopicNode{
	{Key: TopicKeyTech, DisplayName: "科技", Description: "AI、开发者、创业与产业", SortOrder: 10, Enabled: true},
	{Key: TopicKeyTechAI, ParentKey: TopicKeyTech, DisplayName: "AI", Description: "模型、研究与工具", SortOrder: 20, Enabled: true},
	{Key: TopicKeyTechAIModels, ParentKey: TopicKeyTechAI, DisplayName: "模型发布", SortOrder: 30, Selectable: true, Enabled: true},
	{Key: TopicKeyTechAIResearch, ParentKey: TopicKeyTechAI, DisplayName: "研究进展", SortOrder: 40, Selectable: true, Enabled: true},
	{Key: TopicKeyTechAITools, ParentKey: TopicKeyTechAI, DisplayName: "工具与论文", SortOrder: 50, Selectable: true, Enabled: true},
	{Key: TopicKeyTechDev, ParentKey: TopicKeyTech, DisplayName: "开发者与开源", SortOrder: 60, Selectable: true, Enabled: true},
	{Key: TopicKeyTechStartups, ParentKey: TopicKeyTech, DisplayName: "创业与产业", SortOrder: 70, Selectable: true, Enabled: true},
	{Key: "art", DisplayName: "艺术", Description: "即将上线", SortOrder: 80, Enabled: false},
	{Key: "music", DisplayName: "音乐", Description: "即将上线", SortOrder: 90, Enabled: false},
	{Key: "politics", DisplayName: "时政", Description: "即将上线", SortOrder: 100, Enabled: false},
}

var topicNodeByKey = func() map[string]TopicNode {
	result := make(map[string]TopicNode, len(topicCatalogFlat))
	for _, node := range topicCatalogFlat {
		result[node.Key] = node
	}
	return result
}()

func TopicCatalogFlat() []TopicNode {
	return append([]TopicNode(nil), topicCatalogFlat...)
}

func TopicCatalogTree() []TopicNode {
	childrenByParent := make(map[string][]TopicNode)
	for _, node := range topicCatalogFlat {
		childrenByParent[node.ParentKey] = append(childrenByParent[node.ParentKey], node)
	}
	var build func(parentKey string) []TopicNode
	build = func(parentKey string) []TopicNode {
		children := append([]TopicNode(nil), childrenByParent[parentKey]...)
		sort.Slice(children, func(i, j int) bool {
			return children[i].SortOrder < children[j].SortOrder
		})
		result := make([]TopicNode, 0, len(children))
		for _, child := range children {
			child.Children = build(child.Key)
			result = append(result, child)
		}
		return result
	}
	return build("")
}

func IsTopicKeyKnown(key string) bool {
	_, ok := topicNodeByKey[key]
	return ok
}

func IsLeafTopicKey(key string) bool {
	node, ok := topicNodeByKey[key]
	return ok && node.Selectable
}

func IsTopicKeySelectable(key string) bool {
	node, ok := topicNodeByKey[key]
	return ok && node.Selectable && node.Enabled
}

func TopicDisplayName(key string) string {
	node, ok := topicNodeByKey[key]
	if !ok {
		return key
	}
	return node.DisplayName
}

func TopicBreadcrumb(key string) string {
	node, ok := topicNodeByKey[key]
	if !ok {
		return key
	}
	parts := []string{node.DisplayName}
	parentKey := node.ParentKey
	for parentKey != "" {
		parent := topicNodeByKey[parentKey]
		parts = append([]string{parent.DisplayName}, parts...)
		parentKey = parent.ParentKey
	}
	return strings.Join(parts, " · ")
}

func LeafTopicKeys() []string {
	keys := make([]string, 0)
	for _, node := range topicCatalogFlat {
		if node.Selectable && node.Enabled {
			keys = append(keys, node.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

func SourceKeysForTopics(topics []string) []string {
	allowed := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if IsTopicKeySelectable(topic) {
			allowed[topic] = struct{}{}
		}
	}
	keys := make([]string, 0)
	seen := make(map[string]struct{})
	for _, spec := range DefaultSourceFeedSpecs() {
		if _, ok := allowed[spec.Topic]; !ok {
			continue
		}
		if _, dup := seen[spec.Key]; dup {
			continue
		}
		seen[spec.Key] = struct{}{}
		keys = append(keys, spec.Key)
	}
	sort.Strings(keys)
	return keys
}

func TopicHasSources(key string) bool {
	for _, spec := range DefaultSourceFeedSpecs() {
		if spec.Topic == key {
			return true
		}
	}
	return false
}
```

```go
func DefaultSourceFeedSpecs() []SourceFeedSpec {
	return []SourceFeedSpec{
		{Key: SourceKeyHackerNews, Topic: TopicKeyTechDev, Format: SourceFeedFormatRSS, URL: "https://news.ycombinator.com/rss"},
		{Key: SourceKeyGitHubTrending, Topic: TopicKeyTechDev, Format: SourceFeedFormatRSS, URL: "https://cdn.jsdelivr.net/gh/isboyjc/github-trending-api/data/daily/all.xml"},
		{Key: SourceKeyArxivCSAI, Topic: TopicKeyTechAIResearch, Format: SourceFeedFormatAtom, URL: "https://export.arxiv.org/rss/cs.AI"},
		{Key: SourceKeyArxivCSCL, Topic: TopicKeyTechAIResearch, Format: SourceFeedFormatAtom, URL: "https://export.arxiv.org/rss/cs.CL"},
		{Key: SourceKeyArxivCSLG, Topic: TopicKeyTechAIResearch, Format: SourceFeedFormatAtom, URL: "https://export.arxiv.org/rss/cs.LG"},
		{Key: SourceKeyPapersWithCode, Topic: TopicKeyTechAITools, Format: SourceFeedFormatHTML, URL: "https://huggingface.co/papers", HTMLParser: "huggingface-papers"},
		{Key: SourceKeyOpenAIBlog, Topic: TopicKeyTechAIModels, Format: SourceFeedFormatRSS, URL: "https://openai.com/blog/rss.xml"},
		{Key: SourceKeyAnthropicBlog, Topic: TopicKeyTechAIModels, Format: SourceFeedFormatRSS, URL: "https://www.anthropic.com/rss.xml"},
		{Key: SourceKeyGoogleDeepMind, Topic: TopicKeyTechAIModels, Format: SourceFeedFormatRSS, URL: "https://deepmind.google/blog/rss.xml"},
		{Key: SourceKeyMetaAIBlog, Topic: TopicKeyTechAIModels, Format: SourceFeedFormatHTML, URL: "https://ai.meta.com/blog/", HTMLParser: "meta-ai"},
		{Key: SourceKeyTheDecoder, Topic: TopicKeyTechStartups, Format: SourceFeedFormatRSS, URL: "https://the-decoder.com/feed/"},
		{Key: SourceKeyVentureBeatAI, Topic: TopicKeyTechStartups, Format: SourceFeedFormatRSS, URL: "https://venturebeat.com/category/ai/feed"},
		{Key: SourceKeyTechCrunchAI, Topic: TopicKeyTechStartups, Format: SourceFeedFormatRSS, URL: "https://techcrunch.com/category/artificial-intelligence/feed/"},
	}
}
```

- [ ] **Step 4: Run the domain tests to verify they pass**

Run:

```bash
go test ./internal/app/dailybrief/domain -count=1
```

Expected: PASS, including the new tree and source-expansion tests.

- [ ] **Step 5: Commit**

```bash
git add internal/app/dailybrief/domain/topic_catalog.go internal/app/dailybrief/domain/source_feed_catalog.go internal/app/dailybrief/domain/constants_test.go internal/app/dailybrief/domain/topic_catalog_test.go internal/app/dailybrief/domain/source_feed_catalog_test.go
git commit -m "feat: add daily brief topic tree domain model"
```

### Task 2: Derive Source Snapshots in Services and Preserve Runtime Stability

**Files:**
- Modify: `internal/app/dailybrief/service/subscription_service.go`
- Modify: `internal/app/dailybrief/service/subscription_service_test.go`
- Create: `internal/app/dailybrief/service/subscription_snapshot_service.go`
- Create: `internal/app/dailybrief/service/subscription_snapshot_service_test.go`
- Modify: `internal/app/dailybrief/service/candidate_pipeline.go`
- Modify: `internal/app/dailybrief/service/prompt_builder.go`
- Modify: `internal/app/dailybrief/service/prompt_builder_test.go`

- [ ] **Step 1: Write failing service tests for source derivation and snapshot recompute**

```go
func TestSubscriptionServiceUpsertDerivesSourcesFromTopics(t *testing.T) {
	repo := &stubSubscriptionRepo{}
	service := NewSubscriptionService(repo)

	subscription := domain.Subscription{
		UserID:            "user-1",
		Enabled:           true,
		Timezone:          "UTC",
		DeliveryTimeLocal: "08:00",
		Topics:            []string{domain.TopicKeyTechAIModels, domain.TopicKeyTechDev},
		Sources:           []string{"user-supplied-value"},
	}

	_, err := service.Upsert(context.Background(), subscription)
	if err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	want := []string{
		domain.SourceKeyAnthropicBlog,
		domain.SourceKeyGitHubTrending,
		domain.SourceKeyGoogleDeepMind,
		domain.SourceKeyHackerNews,
		domain.SourceKeyMetaAIBlog,
		domain.SourceKeyOpenAIBlog,
	}
	if !reflect.DeepEqual(repo.lastUpsert.Sources, want) {
		t.Fatalf("unexpected derived sources: got %v want %v", repo.lastUpsert.Sources, want)
	}
}

func TestSubscriptionServiceRejectsDisabledOrNonLeafTopics(t *testing.T) {
	service := NewSubscriptionService(&stubSubscriptionRepo{})

	_, err := service.Upsert(context.Background(), domain.Subscription{
		UserID:            "user-1",
		Enabled:           true,
		Timezone:          "UTC",
		DeliveryTimeLocal: "08:00",
		Topics:            []string{"tech"},
	})
	if err == nil || !strings.Contains(err.Error(), "selectable leaf") {
		t.Fatalf("expected non-leaf topic error, got %v", err)
	}
}

func TestSubscriptionSnapshotServiceRefreshesStoredSources(t *testing.T) {
	repo := &snapshotRepo{
		subscriptions: []domain.Subscription{
			domain.NewSubscription("user-1", "UTC", "08:00", []string{domain.TopicKeyTechStartups}, nil),
		},
	}
	service := NewSubscriptionSnapshotService(repo)

	result, err := service.Recompute(context.Background(), SubscriptionSnapshotRecomputeInput{})
	if err != nil {
		t.Fatalf("unexpected recompute error: %v", err)
	}
	if result.UpdatedCount != 1 {
		t.Fatalf("expected one updated subscription, got %+v", result)
	}
	if len(repo.upserts) != 1 || len(repo.upserts[0].Sources) == 0 {
		t.Fatalf("expected derived sources to be persisted, got %+v", repo.upserts)
	}
}
```

- [ ] **Step 2: Run the service tests to verify they fail**

Run:

```bash
go test ./internal/app/dailybrief/service -run "TestSubscriptionServiceUpsertDerivesSourcesFromTopics|TestSubscriptionServiceRejectsDisabledOrNonLeafTopics|TestSubscriptionSnapshotServiceRefreshesStoredSources|TestBuildBriefGenerationPromptIncludesTopicTitles" -count=1
```

Expected: FAIL because `Upsert` still trusts caller-provided `sources`, no snapshot recompute service exists, and prompt/candidate code still uses old topic keys.

- [ ] **Step 3: Implement save-time source derivation, recompute service, and runtime key updates**

```go
func (s *SubscriptionService) Upsert(ctx context.Context, subscription domain.Subscription) (domain.Subscription, error) {
	normalized := normalizeSubscription(subscription)
	derived, err := deriveSubscriptionSnapshot(normalized)
	if err != nil {
		return domain.Subscription{}, err
	}
	if err := validateSubscription(derived); err != nil {
		return domain.Subscription{}, err
	}
	return s.repo.Upsert(ctx, derived)
}

func deriveSubscriptionSnapshot(subscription domain.Subscription) (domain.Subscription, error) {
	derived := subscription
	for _, topic := range derived.Topics {
		if !domain.IsTopicKeySelectable(topic) {
			return domain.Subscription{}, fmt.Errorf("subscription topic %q must be an enabled selectable leaf", topic)
		}
	}
	derived.Sources = domain.SourceKeysForTopics(derived.Topics)
	if derived.Enabled && len(derived.Topics) > 0 && len(derived.Sources) == 0 {
		return domain.Subscription{}, fmt.Errorf("subscription topics %v do not currently resolve to any sources", derived.Topics)
	}
	return derived, nil
}
```

```go
type SubscriptionSnapshotRecomputeInput struct {
	UserIDs []string
}

type SubscriptionSnapshotRecomputeResult struct {
	ScannedCount int
	UpdatedCount int
}

type SubscriptionSnapshotService struct {
	repo port.SubscriptionRepository
}

func NewSubscriptionSnapshotService(repo port.SubscriptionRepository) *SubscriptionSnapshotService {
	return &SubscriptionSnapshotService{repo: repo}
}

func (s *SubscriptionSnapshotService) Recompute(ctx context.Context, input SubscriptionSnapshotRecomputeInput) (SubscriptionSnapshotRecomputeResult, error) {
	subs, err := s.repo.List(ctx, port.SubscriptionListFilter{})
	if err != nil {
		return SubscriptionSnapshotRecomputeResult{}, err
	}
	result := SubscriptionSnapshotRecomputeResult{ScannedCount: len(subs)}
	filter := make(map[string]struct{}, len(input.UserIDs))
	for _, userID := range input.UserIDs {
		filter[strings.TrimSpace(userID)] = struct{}{}
	}
	for _, subscription := range subs {
		if len(filter) > 0 {
			if _, ok := filter[subscription.UserID]; !ok {
				continue
			}
		}
		updated, err := deriveSubscriptionSnapshot(normalizeSubscription(subscription))
		if err != nil {
			return SubscriptionSnapshotRecomputeResult{}, err
		}
		if reflect.DeepEqual(updated.Sources, subscription.Sources) {
			continue
		}
		if _, err := s.repo.Upsert(ctx, updated); err != nil {
			return SubscriptionSnapshotRecomputeResult{}, err
		}
		result.UpdatedCount++
	}
	return result, nil
}
```

```go
func inferTopic(source string) string {
	switch source {
	case domain.SourceKeyArxivCSAI, domain.SourceKeyArxivCSCL, domain.SourceKeyArxivCSLG:
		return domain.TopicKeyTechAIResearch
	case domain.SourceKeyOpenAIBlog, domain.SourceKeyAnthropicBlog, domain.SourceKeyGoogleDeepMind, domain.SourceKeyMetaAIBlog:
		return domain.TopicKeyTechAIModels
	case domain.SourceKeyGitHubTrending, domain.SourceKeyHackerNews:
		return domain.TopicKeyTechDev
	case domain.SourceKeyPapersWithCode:
		return domain.TopicKeyTechAITools
	case domain.SourceKeyTheDecoder, domain.SourceKeyVentureBeatAI, domain.SourceKeyTechCrunchAI:
		return domain.TopicKeyTechStartups
	default:
		return domain.TopicKeyTechAIResearch
	}
}
```

```go
func BuildBriefGenerationPrompt(briefDate string, candidates []domain.Candidate) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "简报日期：%s\n", strings.TrimSpace(briefDate))
	fmt.Fprintf(&builder, "候选条目（%d 条）：\n", len(candidates))
	for index, candidate := range candidates {
		publishedAt := ""
		if !candidate.PublishedAt.IsZero() {
			publishedAt = candidate.PublishedAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(
			&builder,
			"%d. title=%q url=%q source=%q topic=%q publishedAt=%s summary=%q\n",
			index+1,
			candidate.Title,
			candidate.URL,
			candidate.Source,
			candidate.Topic,
			publishedAt,
			candidate.SummarySnippet,
		)
	}
	builder.WriteString("\n栏目中文标题参考：\n")
	for _, key := range domain.LeafTopicKeys() {
		fmt.Fprintf(&builder, "- %s: %s\n", key, domain.TopicBreadcrumb(key))
	}
	return builder.String()
}
```

- [ ] **Step 4: Run the service tests to verify they pass**

Run:

```bash
go test ./internal/app/dailybrief/service -count=1
```

Expected: PASS, including derivation, recompute, and prompt tests.

- [ ] **Step 5: Commit**

```bash
git add internal/app/dailybrief/service/subscription_service.go internal/app/dailybrief/service/subscription_service_test.go internal/app/dailybrief/service/subscription_snapshot_service.go internal/app/dailybrief/service/subscription_snapshot_service_test.go internal/app/dailybrief/service/candidate_pipeline.go internal/app/dailybrief/service/prompt_builder.go internal/app/dailybrief/service/prompt_builder_test.go
git commit -m "feat: derive daily brief source snapshots from topics"
```

### Task 3: Expose Topic Catalog and Admin Snapshot Recompute APIs

**Files:**
- Create: `internal/app/dailybrief/service/topic_catalog_service.go`
- Create: `internal/app/dailybrief/service/topic_catalog_service_test.go`
- Modify: `internal/adapter/http/dailybrief/dto.go`
- Modify: `internal/adapter/http/dailybrief/handler.go`
- Modify: `internal/adapter/http/dailybrief/routes.go`
- Modify: `internal/adapter/http/dailybrief/handler_test.go`
- Modify: `internal/bootstrap/dailybrief/runtime.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Write failing HTTP tests for topic catalog, subscription contract, and recompute**

```go
func TestHandlerGetTopicCatalogReturnsRecursiveNodes(t *testing.T) {
	router := newDailyBriefRouter(
		dailybriefservice.NewReadService(&handlerSubscriptionRepo{}, &handlerIssueRepo{}, &handlerItemRepo{}),
		dailybriefservice.NewSubscriptionService(&handlerSubscriptionRepo{}),
		dailybriefservice.NewTopicCatalogService(),
		nil,
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

func TestHandlerUpdateSubscriptionIgnoresClientSources(t *testing.T) {
	repo := &handlerSubscriptionRepo{}
	router := newDailyBriefRouter(
		dailybriefservice.NewReadService(repo, &handlerIssueRepo{}, &handlerItemRepo{}),
		dailybriefservice.NewSubscriptionService(repo),
		dailybriefservice.NewTopicCatalogService(),
		nil,
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
```

- [ ] **Step 2: Run the HTTP tests to verify they fail**

Run:

```bash
go test ./internal/adapter/http/dailybrief -run "TestHandlerGetTopicCatalogReturnsRecursiveNodes|TestHandlerUpdateSubscriptionIgnoresClientSources|TestAdminRecomputeRouteTriggersSnapshotRefresh" -count=1
```

Expected: FAIL because the topic-catalog route and recompute endpoint do not exist and the subscription DTO still accepts `sources`.

- [ ] **Step 3: Implement the read model service, user API changes, and admin recompute route**

```go
type TopicCatalogNodeReadModel struct {
	Key         string                     `json:"key"`
	DisplayName string                     `json:"displayName"`
	Description string                     `json:"description"`
	Selectable  bool                       `json:"selectable"`
	Enabled     bool                       `json:"enabled"`
	HasSources  bool                       `json:"hasSources,omitempty"`
	Children    []TopicCatalogNodeReadModel `json:"children"`
}

type TopicCatalogService struct{}

func NewTopicCatalogService() *TopicCatalogService { return &TopicCatalogService{} }

func (s *TopicCatalogService) GetTree() []TopicCatalogNodeReadModel {
	var mapNode func(node domain.TopicNode) TopicCatalogNodeReadModel
	mapNode = func(node domain.TopicNode) TopicCatalogNodeReadModel {
		children := make([]TopicCatalogNodeReadModel, 0, len(node.Children))
		for _, child := range node.Children {
			children = append(children, mapNode(child))
		}
		return TopicCatalogNodeReadModel{
			Key:         node.Key,
			DisplayName: node.DisplayName,
			Description: node.Description,
			Selectable:  node.Selectable,
			Enabled:     node.Enabled,
			HasSources:  node.Selectable && domain.TopicHasSources(node.Key),
			Children:    children,
		}
	}

	tree := domain.TopicCatalogTree()
	result := make([]TopicCatalogNodeReadModel, 0, len(tree))
	for _, node := range tree {
		result = append(result, mapNode(node))
	}
	return result
}
```

```go
type subscriptionResponse struct {
	Enabled           bool     `json:"enabled"`
	Timezone          string   `json:"timezone"`
	DeliveryTimeLocal string   `json:"deliveryTimeLocal"`
	Topics            []string `json:"topics"`
}

type updateSubscriptionRequest struct {
	Enabled           bool     `json:"enabled"`
	Timezone          string   `json:"timezone"`
	DeliveryTimeLocal string   `json:"deliveryTimeLocal"`
	Topics            []string `json:"topics"`
}

type recomputeSnapshotsRequest struct {
	UserIDs []string `json:"userIds"`
}
```

```go
type Handler struct {
	readService              *dailybriefservice.ReadService
	subscriptionService      *dailybriefservice.SubscriptionService
	topicCatalogService      *dailybriefservice.TopicCatalogService
	subscriptionSnapshotService *dailybriefservice.SubscriptionSnapshotService
}

func (h *Handler) GetTopicCatalog(c *gin.Context) {
	writeSuccess(c, gin.H{"nodes": h.topicCatalogService.GetTree()})
}

func (h *Handler) RecomputeSubscriptionSnapshots(c *gin.Context) {
	var req recomputeSnapshotsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	result, err := h.subscriptionSnapshotService.Recompute(c.Request.Context(), dailybriefservice.SubscriptionSnapshotRecomputeInput{UserIDs: req.UserIDs})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, result)
}
```

```go
func RegisterRoutes(r gin.IRoutes, readService *dailybriefservice.ReadService, subscriptionService *dailybriefservice.SubscriptionService, topicCatalogService *dailybriefservice.TopicCatalogService) {
	handler := NewHandler(readService, subscriptionService, topicCatalogService, nil)
	r.GET("/daily-brief/topic-catalog", handler.GetTopicCatalog)
	r.GET("/daily-brief/today", handler.GetToday)
	r.GET("/daily-brief/issues", handler.GetIssueByDate)
	r.GET("/daily-brief/subscription", handler.GetSubscription)
	r.PUT("/daily-brief/subscription", handler.UpdateSubscription)
}

func RegisterAdminRoutes(r gin.IRoutes, snapshotService *dailybriefservice.SubscriptionSnapshotService) {
	handler := NewHandler(nil, nil, nil, snapshotService)
	r.POST("/daily-brief/subscription-snapshots/recompute", handler.RecomputeSubscriptionSnapshots)
}
```

```go
runtime.TopicCatalogService = dailybriefservice.NewTopicCatalogService()
runtime.SubscriptionSnapshotService = dailybriefservice.NewSubscriptionSnapshotService(subscriptionRepo)
```

```go
protected := resolveContextPath(r, cfg).Group("/")
protected.Use(umw.RequireLogin())
dailybriefhttp.RegisterRoutes(protected, runtime.ReadService, runtime.SubscriptionService, runtime.TopicCatalogService)

admin := resolveContextPath(r, cfg).Group("/")
admin.Use(umw.RequireLogin(), umw.RequireRole("admin"))
dailybriefhttp.RegisterAdminRoutes(admin, runtime.SubscriptionSnapshotService)
```

- [ ] **Step 4: Run the HTTP tests to verify they pass**

Run:

```bash
go test ./internal/adapter/http/dailybrief -count=1
go test ./internal/bootstrap/dailybrief -run TestDailyBrief -count=1
```

Expected: PASS for HTTP route tests; bootstrap tests compile and pass with the new runtime wiring.

- [ ] **Step 5: Commit**

```bash
git add internal/app/dailybrief/service/topic_catalog_service.go internal/app/dailybrief/service/topic_catalog_service_test.go internal/adapter/http/dailybrief/dto.go internal/adapter/http/dailybrief/handler.go internal/adapter/http/dailybrief/routes.go internal/adapter/http/dailybrief/handler_test.go internal/bootstrap/dailybrief/runtime.go cmd/server/main.go
git commit -m "feat: add daily brief topic catalog and snapshot admin APIs"
```

### Task 4: Migrate the Frontend to a Catalog-Driven Topic-Only Subscription UI

**Files:**
- Modify: `frontend/src/types/dailyBrief.ts`
- Modify: `frontend/src/services/dailyBriefService.ts`
- Create: `frontend/src/lib/dailyBriefTopicCatalog.ts`
- Modify: `frontend/src/pages/DailyBriefPage.tsx`

- [ ] **Step 1: Refactor the frontend types and services to match the new API contract**

```ts
export interface DailyBriefTopicCatalogNode {
  key: string;
  displayName: string;
  description: string;
  selectable: boolean;
  enabled: boolean;
  hasSources?: boolean;
  children: DailyBriefTopicCatalogNode[];
}

export interface DailyBriefTopicCatalogResponse {
  nodes: DailyBriefTopicCatalogNode[];
}

export interface DailyBriefSubscription {
  enabled: boolean;
  timezone: string;
  deliveryTimeLocal: string;
  topics: string[];
}

export interface UpdateDailyBriefSubscriptionInput {
  enabled: boolean;
  timezone: string;
  deliveryTimeLocal: string;
  topics: string[];
}
```

```ts
export async function getDailyBriefTopicCatalog(): Promise<DailyBriefTopicCatalogResponse> {
  return api.get<DailyBriefTopicCatalogResponse>("/daily-brief/topic-catalog");
}

function buildDefaultSubscription(): DailyBriefSubscription {
  return {
    enabled: false,
    timezone: getLocalTimezone(),
    deliveryTimeLocal: FALLBACK_DELIVERY_TIME,
    topics: []
  };
}

export async function updateDailyBriefSubscription(
  input: UpdateDailyBriefSubscriptionInput
): Promise<DailyBriefSubscription> {
  return api.put<DailyBriefSubscription>("/daily-brief/subscription", input);
}
```

- [ ] **Step 2: Add frontend catalog helpers and update the page to remove source checkboxes**

```ts
import type { DailyBriefTopicCatalogNode } from "@/types/dailyBrief";

export function flattenTopicCatalog(nodes: DailyBriefTopicCatalogNode[]): DailyBriefTopicCatalogNode[] {
  const result: DailyBriefTopicCatalogNode[] = [];
  const walk = (node: DailyBriefTopicCatalogNode) => {
    result.push(node);
    node.children.forEach(walk);
  };
  nodes.forEach(walk);
  return result;
}

export function topicBreadcrumbMap(nodes: DailyBriefTopicCatalogNode[]): Record<string, string> {
  const result: Record<string, string> = {};
  const walk = (node: DailyBriefTopicCatalogNode, parents: string[]) => {
    const next = [...parents, node.displayName];
    result[node.key] = next.join(" · ");
    node.children.forEach((child) => walk(child, next));
  };
  nodes.forEach((node) => walk(node, []));
  return result;
}
```

```tsx
const [topicCatalog, setTopicCatalog] = React.useState<DailyBriefTopicCatalogNode[]>([]);

const loadTopicCatalog = React.useCallback(async () => {
  const result = await getDailyBriefTopicCatalog();
  setTopicCatalog(result.nodes);
}, []);

React.useEffect(() => {
  void loadTopicCatalog();
}, [loadTopicCatalog]);

const onSaveSubscription = async (event: React.FormEvent<HTMLFormElement>) => {
  event.preventDefault();
  if (!subscriptionDraft) return;
  setSavingSubscription(true);
  try {
    const updated = await updateDailyBriefSubscription({
      enabled: subscriptionDraft.enabled,
      timezone: subscriptionDraft.timezone,
      deliveryTimeLocal: subscriptionDraft.deliveryTimeLocal,
      topics: subscriptionDraft.topics
    });
    setSubscription(updated);
    setSubscriptionDraft(updated);
    toast.success("订阅设置已保存。");
  } finally {
    setSavingSubscription(false);
  }
};
```

```tsx
{topicCatalog.map((group) => (
  <div key={group.key} className="space-y-3">
    <div>
      <p className="text-sm font-semibold text-slate-900">{group.displayName}</p>
      <p className="text-xs text-slate-500">{group.description}</p>
    </div>
    <div className="grid gap-2">
      {group.children.map((topic) => {
        const disabled = !topic.selectable || !topic.enabled || topic.hasSources === false;
        return (
          <label key={topic.key} className={`flex items-center gap-2 text-sm ${disabled ? "text-slate-400" : "text-slate-700"}`}>
            <Checkbox
              checked={subscriptionDraft.topics.includes(topic.key)}
              disabled={disabled}
              onCheckedChange={(checked) => onToggleTopic(topic.key, checked === true)}
            />
            <span>{topic.displayName}</span>
            {disabled ? <span className="text-xs">即将上线</span> : null}
          </label>
        );
      })}
    </div>
  </div>
))}
```

- [ ] **Step 3: Run frontend verification to catch contract mismatches**

Run:

```bash
npm --prefix frontend run lint
npm --prefix frontend run build
```

Expected: PASS. The build should succeed without any remaining references to `subscription.sources`, `DAILY_BRIEF_SOURCE_OPTIONS`, or hard-coded topic option arrays.

- [ ] **Step 4: Manually smoke-test the `/brief` page against the local server**

Run:

```bash
npm --prefix frontend run dev
```

Manual checks:

- `/brief` loads without crashing
- topic groups render from `GET /daily-brief/topic-catalog`
- disabled topics cannot be selected
- saving a subscription sends only `enabled`, `timezone`, `deliveryTimeLocal`, and `topics`
- daily brief issue cards still render topic/source badges for generated items

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types/dailyBrief.ts frontend/src/services/dailyBriefService.ts frontend/src/lib/dailyBriefTopicCatalog.ts frontend/src/pages/DailyBriefPage.tsx
git commit -m "feat: drive daily brief subscription UI from topic catalog"
```

### Task 5: Update Integration Coverage and Run the Full Verification Set

**Files:**
- Modify: `internal/bootstrap/dailybrief/e2e_integration_test.go`

- [ ] **Step 1: Update the Daily Brief end-to-end test to use the new subscription contract**

```go
putBody := bytes.NewBufferString(`{
  "enabled": true,
  "timezone": "UTC",
  "deliveryTimeLocal": "08:00",
  "topics": ["tech.ai.models", "tech.dev"]
}`)
putReq := httptest.NewRequest(http.MethodPut, "/daily-brief/subscription", putBody)
putReq.Header.Set("Content-Type", "application/json")
```

```go
if !slices.Contains(saved.Subscription.Topics, "tech.ai.models") {
	t.Fatalf("expected hierarchical topic to persist, got %+v", saved.Subscription.Topics)
}
if len(saved.Subscription.Sources) == 0 {
	t.Fatalf("expected derived sources snapshot, got %+v", saved.Subscription)
}
```

- [ ] **Step 2: Run the focused backend suites**

Run:

```bash
go test ./internal/app/dailybrief/domain ./internal/app/dailybrief/service ./internal/adapter/http/dailybrief ./internal/bootstrap/dailybrief -count=1
```

Expected: PASS across domain, service, HTTP, and bootstrap coverage.

- [ ] **Step 3: Run the repository-wide Daily Brief verification set**

Run:

```bash
go test ./internal/app/dailybrief/... ./internal/adapter/http/dailybrief/... ./internal/bootstrap/dailybrief/... -count=1
npm --prefix frontend run lint
npm --prefix frontend run build
```

Expected: PASS. No stale references to the old flat topic keys should remain in the touched paths except where legacy issue data is intentionally tolerated.

- [ ] **Step 4: Run optional integration coverage when the environment is available**

Run:

```bash
$env:RAG_INTEGRATION_DAILY_BRIEF="1"; go test ./internal/bootstrap/dailybrief -run TestDailyBrief -count=1 -v
```

Expected: PASS when the integration environment is configured; otherwise the test may `Skip`, which is acceptable and should be noted in the execution log.

- [ ] **Step 5: Commit**

```bash
git add internal/bootstrap/dailybrief/e2e_integration_test.go
git commit -m "test: cover daily brief topic catalog rollout end to end"
```

## Self-Review Checklist

- Spec coverage:
  - topic-first subscription flow: covered by Tasks 1-4
  - persisted `sources` execution snapshot: covered by Task 2
  - topic catalog API: covered by Task 3
  - admin-only explicit recompute path: covered by Task 3
  - frontend removal of source selection UI: covered by Task 4
  - verification and integration coverage: covered by Task 5
- Placeholder scan:
  - no `TODO`, `TBD`, or “implement later” markers remain
  - every task lists concrete files, commands, and code targets
- Type consistency:
  - `topics[]` remains the editable user field
  - `sources[]` remains persisted backend-derived snapshot state
  - route names stay consistent: `/daily-brief/topic-catalog`, `/daily-brief/subscription`, `/daily-brief/subscription-snapshots/recompute`
