package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentstate "local/rag-project/internal/app/agent/state"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

const (
	maxPlannerSearchResults = 3
	maxPlannerFetchPages    = 2
	maxPlannerEvidenceItems = 3
	maxPlannerDomains       = 5
	maxPlannerCleanTextLen  = 1200
)

var allowedReasons = map[string]struct{}{
	"fetched_readable_evidence":  {},
	"need_more_sources":          {},
	"fetch_failed_retryable":     {},
	"no_new_fetchable_urls":      {},
	"no_progress_across_rounds":  {},
	"iteration_budget_exhausted": {},
	"results_low_quality":        {},
}

type LLMPlanner struct {
	chatService aichat.LLMService
}

func NewLLMPlanner(chatService aichat.LLMService) *LLMPlanner {
	if chatService == nil {
		return nil
	}
	return &LLMPlanner{chatService: chatService}
}

func (p *LLMPlanner) Plan(ctx context.Context, input PlanInput) (PlanResult, error) {
	if p == nil || p.chatService == nil || input.Session == nil {
		return PlanResult{}, nil
	}

	summary := BuildSummary(input)
	payload, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return PlanResult{}, fmt.Errorf("marshal planner summary: %w", err)
	}

	jsonMode := true
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(plannerSystemPrompt),
			convention.UserMessage("Planner input:\n" + string(payload) + "\n\nReturn JSON only."),
		},
		JSONMode: &jsonMode,
	}

	response, err := p.chatService.ChatWithRequest(request)
	if err != nil {
		return PlanResult{}, fmt.Errorf("llm planner call: %w", err)
	}

	result, err := parsePlanResult(response)
	if err != nil {
		return PlanResult{}, err
	}
	if err := validatePlanResult(result, summary); err != nil {
		return PlanResult{}, err
	}
	return result, nil
}

type plannerResponse struct {
	Decision   string     `json:"decision"`
	Reason     string     `json:"reason"`
	Confidence float64    `json:"confidence"`
	NextQuery  string     `json:"next_query"`
	Preferred  []string   `json:"preferred_urls"`
	Avoid      []string   `json:"avoid_urls"`
	AnswerPlan answerPlan `json:"answer_plan"`
	Notes      []string   `json:"notes"`
}

type answerPlan struct {
	UseEvidenceIDs []string `json:"use_evidence_ids"`
	AnswerStyle    string   `json:"answer_style"`
}

func parsePlanResult(raw string) (PlanResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return PlanResult{}, fmt.Errorf("planner response is empty")
	}
	if extracted := extractJSONBlock(raw); extracted != "" {
		raw = extracted
	}

	var parsed plannerResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return PlanResult{}, fmt.Errorf("parse planner json: %w", err)
	}

	return PlanResult{
		Decision:          strings.TrimSpace(parsed.Decision),
		Reason:            strings.TrimSpace(parsed.Reason),
		Confidence:        parsed.Confidence,
		NextQuery:         strings.TrimSpace(parsed.NextQuery),
		PreferredURLs:     uniqueTrimmed(parsed.Preferred),
		AvoidURLs:         uniqueTrimmed(parsed.Avoid),
		AnswerEvidenceIDs: uniqueTrimmed(parsed.AnswerPlan.UseEvidenceIDs),
		Notes:             uniqueTrimmed(parsed.Notes),
	}, nil
}

func validatePlanResult(result PlanResult, summary Summary) error {
	switch result.Decision {
	case "answer", "handoff", "continue", "degrade":
	default:
		return fmt.Errorf("invalid decision: %q", result.Decision)
	}
	if _, ok := allowedReasons[result.Reason]; !ok {
		return fmt.Errorf("invalid reason: %q", result.Reason)
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	allowedActions := make(map[string]struct{}, len(summary.AllowedActions))
	for _, item := range summary.AllowedActions {
		allowedActions[item] = struct{}{}
	}
	if _, ok := allowedActions[result.Decision]; !ok {
		return fmt.Errorf("decision %q is not allowed in output mode %q", result.Decision, summary.OutputMode)
	}

	knownURLs := make(map[string]struct{}, len(summary.KnownURLs))
	for _, item := range summary.KnownURLs {
		knownURLs[item] = struct{}{}
	}
	knownEvidence := make(map[string]struct{}, len(summary.KnownEvidenceIDs))
	for _, item := range summary.KnownEvidenceIDs {
		knownEvidence[item] = struct{}{}
	}

	for _, item := range result.PreferredURLs {
		if _, ok := knownURLs[item]; !ok {
			return fmt.Errorf("preferred url not in known urls: %q", item)
		}
	}
	for _, item := range result.AvoidURLs {
		if _, ok := knownURLs[item]; !ok {
			return fmt.Errorf("avoid url not in known urls: %q", item)
		}
	}
	for _, item := range result.AnswerEvidenceIDs {
		if _, ok := knownEvidence[item]; !ok {
			return fmt.Errorf("answer evidence id not known: %q", item)
		}
	}

	switch result.Decision {
	case "continue":
		if strings.TrimSpace(result.NextQuery) == "" {
			return fmt.Errorf("continue decision requires next_query")
		}
		if summary.StopSignals.IterationBudgetExhausted {
			return fmt.Errorf("continue decision conflicts with exhausted iteration budget")
		}
		if summary.StopSignals.NoFetchableURLs {
			return fmt.Errorf("continue decision conflicts with no fetchable urls")
		}
	case "answer", "handoff":
		if !summary.EvidenceSummary.Sufficient && summary.EvidenceSummary.TotalCount == 0 {
			return fmt.Errorf("%s decision requires available evidence", result.Decision)
		}
	case "degrade":
		if strings.TrimSpace(result.NextQuery) != "" || len(result.PreferredURLs) > 0 || len(result.AvoidURLs) > 0 {
			return fmt.Errorf("degrade decision must not request next-step guidance")
		}
	}
	return nil
}

func extractJSONBlock(raw string) string {
	marker := "```json"
	start := strings.Index(raw, marker)
	if start == -1 {
		marker = "```"
		start = strings.Index(raw, marker)
	}
	if start == -1 {
		return ""
	}
	contentStart := strings.IndexByte(raw[start:], '\n')
	if contentStart == -1 {
		return ""
	}
	contentStart += start + 1
	end := strings.Index(raw[contentStart:], "```")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(raw[contentStart : contentStart+end])
}

func uniqueTrimmed(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

const plannerSystemPrompt = `You are the planning layer of an agent runtime.

Choose exactly one next action from the provided allowed_actions list.

You must base your decision only on the provided runtime state.
Do not invent evidence, URLs, domains, or search results.

Rules:
1. Prefer the terminal answer-like action in allowed_actions if the evidence is already sufficient.
2. Prefer "continue" only if another round has realistic remaining value.
3. Prefer "degrade" if iteration budget is exhausted, there are no fetchable URLs, or repeated rounds are not producing progress.
4. If recommending "continue", provide exactly one refined next_query.
5. preferred_urls and avoid_urls must come from the provided known URLs.
6. answer_plan.use_evidence_ids must come from the provided evidence IDs.
7. Return strict JSON only.
`

type Summary struct {
	Question         string             `json:"question"`
	CurrentIteration int                `json:"current_iteration"`
	MaxIterations    int                `json:"max_iterations"`
	OutputMode       string             `json:"output_mode,omitempty"`
	BaselineQuery    string             `json:"baseline_query,omitempty"`
	SearchQuery      string             `json:"search_query,omitempty"`
	SearchSummary    SearchSummary      `json:"search_summary"`
	FetchSummary     FetchSummary       `json:"fetch_summary"`
	EvidenceSummary  EvidenceSummary    `json:"evidence_summary"`
	ProgressSummary  ProgressSummary    `json:"progress_summary"`
	PolicySummary    PolicySummary      `json:"policy_summary"`
	StopSignals      StopSignals        `json:"stop_signals"`
	AllowedActions   []string           `json:"allowed_actions"`
	Constraints      PlannerConstraints `json:"constraints"`
	KnownURLs        []string           `json:"known_urls,omitempty"`
	KnownEvidenceIDs []string           `json:"known_evidence_ids,omitempty"`
}

type SearchSummary struct {
	Provider    string              `json:"provider,omitempty"`
	ResultCount int                 `json:"result_count"`
	NewURLCount int                 `json:"new_url_count"`
	Domains     []string            `json:"domains,omitempty"`
	TopResults  []PlannerSearchItem `json:"top_results,omitempty"`
}

type PlannerSearchItem struct {
	ID      string `json:"id,omitempty"`
	Title   string `json:"title,omitempty"`
	URL     string `json:"url,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

type FetchSummary struct {
	FetchCount            int                `json:"fetch_count"`
	ReadableCount         int                `json:"readable_count"`
	DegradedCount         int                `json:"degraded_count"`
	RetryableFailureCount int                `json:"retryable_failure_count"`
	Pages                 []PlannerFetchPage `json:"pages,omitempty"`
}

type PlannerFetchPage struct {
	ID          string `json:"id,omitempty"`
	URL         string `json:"url,omitempty"`
	Status      string `json:"status,omitempty"`
	CleanText   string `json:"clean_text,omitempty"`
	ErrorReason string `json:"error_reason,omitempty"`
}

type EvidenceSummary struct {
	Sufficient bool                  `json:"sufficient"`
	Reason     string                `json:"reason,omitempty"`
	TotalCount int                   `json:"total_count"`
	NewCount   int                   `json:"new_count"`
	Items      []PlannerEvidenceItem `json:"items,omitempty"`
}

type PlannerEvidenceItem struct {
	ID        string `json:"id,omitempty"`
	Source    string `json:"source,omitempty"`
	Content   string `json:"content,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
}

type ProgressSummary struct {
	ProgressKind                string   `json:"progress_kind,omitempty"`
	ContinueCount               int      `json:"continue_count"`
	NewURLCount                 int      `json:"new_url_count"`
	NewEvidenceCount            int      `json:"new_evidence_count"`
	ConsecutiveNoProgressRounds int      `json:"consecutive_no_progress_rounds"`
	SeenURLCount                int      `json:"seen_url_count"`
	RecentSeenURLs              []string `json:"recent_seen_urls,omitempty"`
}

type PolicySummary struct {
	BaselineDecision string `json:"baseline_decision,omitempty"`
	BaselineReason   string `json:"baseline_reason,omitempty"`
}

type StopSignals struct {
	IterationBudgetExhausted bool `json:"iteration_budget_exhausted"`
	NoFetchableURLs          bool `json:"no_fetchable_urls"`
}

type PlannerConstraints struct {
	AllowWebSearch                bool `json:"allow_web_search"`
	MustGroundInEvidence          bool `json:"must_ground_in_evidence"`
	MustNotInventURLs             bool `json:"must_not_invent_urls"`
	MustRespectIterationBudget    bool `json:"must_respect_iteration_budget"`
	MustNotRepeatSeenWithoutCause bool `json:"must_not_repeat_seen_urls_without_reason"`
}

func BuildSummary(input PlanInput) Summary {
	session := input.Session
	snapshot := session.Snapshot

	knownURLs := uniqueTrimmed(append(collectSearchURLs(snapshot.Context.SearchResults), snapshot.Context.SeenURLs...))
	knownEvidenceIDs := make([]string, 0, len(snapshot.Evidence.Items))
	for _, item := range snapshot.Evidence.Items {
		if item.ID != "" {
			knownEvidenceIDs = append(knownEvidenceIDs, item.ID)
		}
	}

	return Summary{
		Question:         firstNonEmpty(session.Request.Question, snapshot.Request.Question),
		CurrentIteration: snapshot.Execution.Iteration,
		MaxIterations:    effectiveMaxIterations(snapshot),
		OutputMode:       effectiveOutputMode(snapshot),
		BaselineQuery:    firstNonEmpty(snapshot.Context.RewrittenQuery, snapshot.Request.Question),
		SearchQuery:      snapshot.Context.SearchQuery,
		SearchSummary: SearchSummary{
			Provider:    firstNonEmpty(snapshot.Context.SearchProviderActual, snapshot.Context.SearchProvider),
			ResultCount: len(snapshot.Context.SearchResults),
			NewURLCount: snapshot.Execution.LastNewURLCount,
			Domains:     summarizeDomains(snapshot.Context.SearchResults),
			TopResults:  summarizeSearchResults(snapshot.Context.SearchResults),
		},
		FetchSummary: FetchSummary{
			FetchCount:            len(snapshot.Context.FetchResults),
			ReadableCount:         countReadableFetchPages(snapshot.Context.FetchResults),
			DegradedCount:         countDegradedFetchPages(snapshot.Context.FetchResults),
			RetryableFailureCount: countRetryableFailures(snapshot.Context.FetchResults),
			Pages:                 summarizeFetchPages(snapshot.Context.FetchResults),
		},
		EvidenceSummary: EvidenceSummary{
			Sufficient: snapshot.Evidence.Sufficient,
			Reason:     snapshot.Evidence.SufficiencyReason,
			TotalCount: len(snapshot.Evidence.Items),
			NewCount:   snapshot.Evidence.NewItemsThisRound,
			Items:      summarizeEvidenceItems(snapshot.Evidence.Items),
		},
		ProgressSummary: ProgressSummary{
			ProgressKind:                snapshot.Execution.LastProgressKind,
			ContinueCount:               snapshot.Execution.ContinueCount,
			NewURLCount:                 snapshot.Execution.LastNewURLCount,
			NewEvidenceCount:            snapshot.Execution.LastNewEvidenceCount,
			ConsecutiveNoProgressRounds: snapshot.Execution.ConsecutiveNoProgressRounds,
			SeenURLCount:                len(snapshot.Context.SeenURLs),
			RecentSeenURLs:              tail(snapshot.Context.SeenURLs, 5),
		},
		PolicySummary: PolicySummary{
			BaselineDecision: input.BaselineDecision,
			BaselineReason:   input.BaselineReason,
		},
		StopSignals: StopSignals{
			IterationBudgetExhausted: snapshot.Execution.Iteration+1 >= effectiveMaxIterations(snapshot),
			NoFetchableURLs:          countFetchableURLs(snapshot.Context.SearchResults, snapshot.Context.SeenURLs, snapshot.Context.FetchResults, snapshot.Context.PreferredURLs, snapshot.Context.AvoidURLs) == 0,
		},
		AllowedActions: allowedActionsForOutputMode(snapshot),
		Constraints: PlannerConstraints{
			AllowWebSearch:                snapshot.Request.RuntimeOptions.AllowWebSearch || session.Request.Options.AllowWebSearch,
			MustGroundInEvidence:          true,
			MustNotInventURLs:             true,
			MustRespectIterationBudget:    true,
			MustNotRepeatSeenWithoutCause: true,
		},
		KnownURLs:        knownURLs,
		KnownEvidenceIDs: uniqueTrimmed(knownEvidenceIDs),
	}
}

func effectiveMaxIterations(snapshot agentstate.StateSnapshot) int {
	if snapshot.Execution.MaxIterations > 0 {
		return snapshot.Execution.MaxIterations
	}
	if snapshot.Request.RuntimeOptions.MaxIterations > 0 {
		return snapshot.Request.RuntimeOptions.MaxIterations
	}
	return 0
}

func effectiveOutputMode(snapshot agentstate.StateSnapshot) string {
	if snapshot.Request.RuntimeOptions.OutputMode != "" {
		return snapshot.Request.RuntimeOptions.OutputMode
	}
	return agentstate.OutputModeFinalAnswer
}

func allowedActionsForOutputMode(snapshot agentstate.StateSnapshot) []string {
	if effectiveOutputMode(snapshot) == agentstate.OutputModeHandoff {
		return []string{"handoff", "continue", "degrade"}
	}
	return []string{"answer", "continue", "degrade"}
}

func summarizeDomains(results []agentstate.SearchResultRef) []string {
	domains := make([]string, 0, len(results))
	for _, item := range results {
		if item.Domain != "" {
			domains = append(domains, item.Domain)
		}
	}
	return tail(uniqueTrimmed(domains), maxPlannerDomains)
}

func summarizeSearchResults(results []agentstate.SearchResultRef) []PlannerSearchItem {
	if len(results) == 0 {
		return nil
	}
	window := lastSearchResults(results, maxPlannerSearchResults)
	items := make([]PlannerSearchItem, 0, len(window))
	for _, item := range window {
		items = append(items, PlannerSearchItem{
			ID:      item.ID,
			Title:   item.Title,
			URL:     item.URL,
			Snippet: truncate(item.Snippet, 180),
		})
	}
	return items
}

func summarizeFetchPages(results []agentstate.FetchResultRef) []PlannerFetchPage {
	if len(results) == 0 {
		return nil
	}
	window := lastFetchResults(results, maxPlannerFetchPages)
	items := make([]PlannerFetchPage, 0, len(window))
	for _, item := range window {
		status := "readable"
		if item.Degraded {
			status = "degraded"
		}
		items = append(items, PlannerFetchPage{
			ID:          item.ID,
			URL:         item.URL,
			Status:      status,
			CleanText:   truncate(item.Text, maxPlannerCleanTextLen),
			ErrorReason: truncate(item.ErrorReason, 200),
		})
	}
	return items
}

func summarizeEvidenceItems(items []agentstate.EvidenceItem) []PlannerEvidenceItem {
	if len(items) == 0 {
		return nil
	}
	window := lastEvidenceItems(items, maxPlannerEvidenceItems)
	result := make([]PlannerEvidenceItem, 0, len(window))
	for _, item := range window {
		result = append(result, PlannerEvidenceItem{
			ID:        item.ID,
			Source:    item.Source,
			Content:   truncate(item.Content, 200),
			SourceRef: item.SourceRef,
		})
	}
	return result
}

func countReadableFetchPages(results []agentstate.FetchResultRef) int {
	count := 0
	for _, item := range results {
		if !item.Degraded && strings.TrimSpace(item.Text) != "" {
			count++
		}
	}
	return count
}

func countDegradedFetchPages(results []agentstate.FetchResultRef) int {
	count := 0
	for _, item := range results {
		if item.Degraded {
			count++
		}
	}
	return count
}

func countRetryableFailures(results []agentstate.FetchResultRef) int {
	count := 0
	for _, item := range results {
		if item.Degraded {
			count++
		}
	}
	return count
}

func collectSearchURLs(results []agentstate.SearchResultRef) []string {
	urls := make([]string, 0, len(results))
	for _, item := range results {
		if strings.TrimSpace(item.URL) != "" {
			urls = append(urls, item.URL)
		}
	}
	return urls
}

func countFetchableURLs(results []agentstate.SearchResultRef, seen []string, fetchResults []agentstate.FetchResultRef, preferred []string, avoid []string) int {
	return len(buildFetchURLList(results, seen, fetchResults, preferred, avoid))
}

func buildFetchURLList(results []agentstate.SearchResultRef, seen []string, fetchResults []agentstate.FetchResultRef, preferred []string, avoid []string) []string {
	searchURLs := collectSearchURLs(results)
	if len(searchURLs) == 0 {
		return nil
	}
	seenSet := make(map[string]struct{}, len(seen))
	for _, item := range seen {
		seenSet[item] = struct{}{}
	}
	avoidSet := make(map[string]struct{}, len(avoid))
	for _, item := range avoid {
		avoidSet[item] = struct{}{}
	}
	retryable := make(map[string]struct{}, len(fetchResults))
	for _, item := range fetchResults {
		if item.Degraded && item.URL != "" {
			retryable[item.URL] = struct{}{}
		}
	}
	known := make(map[string]struct{}, len(searchURLs))
	for _, item := range searchURLs {
		known[item] = struct{}{}
	}
	result := make([]string, 0, len(searchURLs))
	added := make(map[string]struct{}, len(searchURLs))
	appendURL := func(url string) {
		if url == "" {
			return
		}
		if _, ok := known[url]; !ok {
			return
		}
		if _, ok := avoidSet[url]; ok {
			return
		}
		if _, ok := added[url]; ok {
			return
		}
		added[url] = struct{}{}
		result = append(result, url)
	}
	for _, item := range preferred {
		appendURL(item)
	}
	for _, item := range searchURLs {
		if _, ok := avoidSet[item]; ok {
			continue
		}
		if _, ok := seenSet[item]; ok {
			if _, retry := retryable[item]; !retry {
				continue
			}
		}
		appendURL(item)
	}
	return result
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit-3]) + "..."
}

func tail(values []string, limit int) []string {
	if len(values) == 0 || limit <= 0 {
		return nil
	}
	if len(values) <= limit {
		return append([]string(nil), values...)
	}
	return append([]string(nil), values[len(values)-limit:]...)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func lastSearchResults(results []agentstate.SearchResultRef, limit int) []agentstate.SearchResultRef {
	if len(results) <= limit || limit <= 0 {
		return results
	}
	return results[len(results)-limit:]
}

func lastFetchResults(results []agentstate.FetchResultRef, limit int) []agentstate.FetchResultRef {
	if len(results) <= limit || limit <= 0 {
		return results
	}
	return results[len(results)-limit:]
}

func lastEvidenceItems(items []agentstate.EvidenceItem, limit int) []agentstate.EvidenceItem {
	if len(items) <= limit || limit <= 0 {
		return items
	}
	return items[len(items)-limit:]
}
