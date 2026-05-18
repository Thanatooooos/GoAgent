package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	ragtool "local/rag-project/internal/app/rag/tool"
	ragcore "local/rag-project/internal/app/rag/tool/core"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/log"
	aichat "local/rag-project/internal/infra-ai/chat"

	"github.com/cloudwego/eino/compose"
)

const (
	externalEvidenceMaxFetchURLs = 3

	readinessReady        = "ready"
	readinessPartial      = "partial"
	readinessInsufficient = "insufficient"
)

type externalEvidenceState struct {
	Question     string
	SearchQuery  string
	SearchResult ragtool.Result
	SelectedURLs []string
	SourceReview externalSourceReview
	FetchResult  ragtool.Result
	Quality      externalEvidenceQualityAssessment
	Readiness    externalReadinessAssessment
	Results      []ragtool.Result
	LastError    string
}

type externalReadinessAssessment struct {
	Readiness          string   `json:"readiness"`
	Confidence         float64  `json:"confidence"`
	Reasoning          string   `json:"reasoning"`
	AnswerStrategy     string   `json:"answerStrategy"`
	MissingInformation []string `json:"missingInformation"`
	CitedURLs          []string `json:"citedUrls"`
}

type externalSourceReview struct {
	TotalResults        int                       `json:"totalResults"`
	AllowedCount        int                       `json:"allowedCount"`
	NeutralCount        int                       `json:"neutralCount"`
	DeniedCount         int                       `json:"deniedCount"`
	SelectedCount       int                       `json:"selectedCount"`
	DistinctDomains     int                       `json:"distinctDomains"`
	DistinctSourceTypes int                       `json:"distinctSourceTypes"`
	Coverage            string                    `json:"coverage"`
	Notes               []string                  `json:"notes"`
	SelectedSources     []externalSourceSelection `json:"selectedSources"`
	RejectedSources     []externalSourceSelection `json:"rejectedSources"`
}

type externalSourceSelection struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Domain        string   `json:"domain"`
	Policy        string   `json:"policy"`
	SourceType    string   `json:"sourceType"`
	ProviderScore float64  `json:"providerScore"`
	RiskFlags     []string `json:"riskFlags"`
	Reasons       []string `json:"reasons"`
}

type externalEvidenceQualityAssessment struct {
	Quality         string   `json:"quality"`
	Confidence      float64  `json:"confidence"`
	Reasoning       string   `json:"reasoning"`
	SourceDiversity string   `json:"sourceDiversity"`
	Corroboration   string   `json:"corroboration"`
	SuccessfulPages int      `json:"successfulPages"`
	FailedPages     int      `json:"failedPages"`
	EmptyPages      int      `json:"emptyPages"`
	TruncatedPages  int      `json:"truncatedPages"`
	Notes           []string `json:"notes"`
}

type ExternalEvidenceWorkflowTool struct {
	runner compose.Runnable[*externalEvidenceState, *externalEvidenceState]
}

func NewExternalEvidenceWorkflowTool(executor *ragruntime.Executor, chatService aichat.LLMService) (*ExternalEvidenceWorkflowTool, error) {
	if executor == nil {
		return nil, fmt.Errorf("executor with registry is required")
	}

	graph := compose.NewGraph[*externalEvidenceState, *externalEvidenceState]()

	graph.AddLambdaNode("search", compose.InvokableLambda(
		func(ctx context.Context, state *externalEvidenceState) (*externalEvidenceState, error) {
			query := strings.TrimSpace(state.Question)
			if query == "" {
				state.LastError = "question is required"
				log.Warnf("[external_evidence] search skipped: missing question")
				return state, nil
			}
			state.SearchQuery = query
			log.Infof("[external_evidence] search start: question=%q", ragtool.TruncateForLog(query))
			result, err := executor.Execute(ctx, ragtool.Call{
				Name:      "web_search",
				Arguments: map[string]any{"query": query},
			})
			if err != nil {
				state.LastError = err.Error()
				log.Warnf("[external_evidence] search failed: %s", ragtool.TruncateForLog(err.Error()))
				return state, nil
			}
			state.SearchResult = result
			state.Results = append(state.Results, result)
			if !result.Successful() && state.LastError == "" {
				state.LastError = strings.TrimSpace(result.ErrorMessage)
			}
			searchView, _ := ragtool.ViewWebSearchResult(result)
			log.Infof(
				"[external_evidence] search done: provider=%s results=%d allow=%d neutral=%d deny=%d",
				searchView.Provider,
				searchView.ResultCount,
				searchView.AllowedCount,
				searchView.NeutralCount,
				searchView.DeniedCount,
			)
			return state, nil
		},
	))

	graph.AddLambdaNode("select", compose.InvokableLambda(
		func(_ context.Context, state *externalEvidenceState) (*externalEvidenceState, error) {
			view, ok := ragtool.ViewWebSearchResult(state.SearchResult)
			if !ok {
				log.Warnf("[external_evidence] select skipped: web_search result view unavailable")
				return state, nil
			}
			state.SelectedURLs = selectFetchURLs(view, externalEvidenceMaxFetchURLs)
			state.SourceReview = reviewExternalSources(view, state.SelectedURLs)
			log.Infof(
				"[external_evidence] select done: selected=%d coverage=%s domains=%d sourceTypes=%d urls=%s",
				len(state.SelectedURLs),
				state.SourceReview.Coverage,
				state.SourceReview.DistinctDomains,
				state.SourceReview.DistinctSourceTypes,
				summarizeURLs(state.SelectedURLs),
			)
			return state, nil
		},
	))

	graph.AddLambdaNode("fetch", compose.InvokableLambda(
		func(ctx context.Context, state *externalEvidenceState) (*externalEvidenceState, error) {
			if len(state.SelectedURLs) == 0 {
				log.Infof("[external_evidence] fetch skipped: no selected urls")
				return state, nil
			}
			log.Infof("[external_evidence] fetch start: urls=%s", summarizeURLs(state.SelectedURLs))
			result, err := executor.Execute(ctx, ragtool.Call{
				Name:      "web_fetch",
				Arguments: map[string]any{"urls": state.SelectedURLs},
			})
			if err != nil {
				state.LastError = err.Error()
				log.Warnf("[external_evidence] fetch failed: %s", ragtool.TruncateForLog(err.Error()))
				return state, nil
			}
			state.FetchResult = result
			state.Results = append(state.Results, result)
			if !result.Successful() && state.LastError == "" {
				state.LastError = strings.TrimSpace(result.ErrorMessage)
			}
			fetchView, _ := ragtool.ViewWebFetchResult(result)
			log.Infof(
				"[external_evidence] fetch done: success=%d failed=%d truncated=%v",
				fetchView.SuccessCount,
				fetchView.FailCount,
				fetchView.AnyPageTruncated(),
			)
			return state, nil
		},
	))

	graph.AddLambdaNode("assess", compose.InvokableLambda(
		func(ctx context.Context, state *externalEvidenceState) (*externalEvidenceState, error) {
			state.Quality = assessExternalEvidenceQuality(state)
			state.Readiness = assessExternalReadiness(ctx, state, chatService)
			log.Infof(
				"[external_evidence] assess done: quality=%s(%.2f) readiness=%s(%.2f) corroboration=%s",
				state.Quality.Quality,
				state.Quality.Confidence,
				state.Readiness.Readiness,
				state.Readiness.Confidence,
				state.Quality.Corroboration,
			)
			return state, nil
		},
	))

	_ = graph.AddEdge(compose.START, "search")
	_ = graph.AddEdge("search", "select")
	_ = graph.AddEdge("select", "fetch")
	_ = graph.AddEdge("fetch", "assess")
	_ = graph.AddEdge("assess", compose.END)

	runner, err := graph.Compile(context.Background(), compose.WithGraphName("external_evidence_workflow"))
	if err != nil {
		return nil, fmt.Errorf("compile external evidence workflow graph: %w", err)
	}

	return &ExternalEvidenceWorkflowTool{runner: runner}, nil
}

func (t *ExternalEvidenceWorkflowTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "external_evidence_workflow",
		Description: "Deterministic external evidence workflow: web_search -> source-aware selection -> web_fetch -> quality and answer-readiness assessment. Use when the local knowledge base is insufficient and external web evidence is needed.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "question",
				Type:        ragtool.ParamTypeString,
				Description: "The user question to answer with external sources.",
				Required:    true,
			},
		},
	}
}

func (t *ExternalEvidenceWorkflowTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.runner == nil {
		return ragtool.Result{
			Name:         "external_evidence_workflow",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: "external evidence workflow runner is not initialized",
		}, nil
	}

	question := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "question"))
	if question == "" {
		return ragtool.Result{
			Name:         "external_evidence_workflow",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: "question is required",
		}, nil
	}

	log.Infof("[external_evidence] workflow start: question=%q", ragtool.TruncateForLog(question))
	final, err := t.runner.Invoke(ctx, &externalEvidenceState{Question: question})
	if err != nil {
		log.Warnf("[external_evidence] workflow failed: %s", ragtool.TruncateForLog(err.Error()))
		return ragtool.Result{
			Name:         "external_evidence_workflow",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: err.Error(),
		}, nil
	}
	if final.LastError != "" && len(final.Results) == 0 {
		log.Warnf("[external_evidence] workflow failed without results: %s", ragtool.TruncateForLog(final.LastError))
		return ragtool.Result{
			Name:         "external_evidence_workflow",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: final.LastError,
		}, nil
	}

	data := map[string]any{
		"question":             final.Question,
		"searchQuery":          final.SearchQuery,
		"selectedUrls":         final.SelectedURLs,
		"selectedDomains":      extractSourceDomains(final.SourceReview.SelectedSources),
		"selectedSourceTypes":  extractSourceTypes(final.SourceReview.SelectedSources),
		"sourceCoverage":       final.SourceReview.Coverage,
		"sourceDiversity":      final.Quality.SourceDiversity,
		"corroboration":        final.Quality.Corroboration,
		"quality":              final.Quality.Quality,
		"qualityConfidence":    final.Quality.Confidence,
		"qualityReasoning":     final.Quality.Reasoning,
		"readiness":            final.Readiness.Readiness,
		"readinessConfidence":  final.Readiness.Confidence,
		"readinessReasoning":   final.Readiness.Reasoning,
		"answerStrategy":       final.Readiness.AnswerStrategy,
		"missingInformation":   final.Readiness.MissingInformation,
		"citedUrls":            final.Readiness.CitedURLs,
		"workflowName":         "external_evidence_workflow",
		"searchWorkflowStatus": buildExternalEvidenceStatus(final),
		"sourceReview":         buildSourceReviewData(final.SourceReview),
		"qualityAssessment":    buildQualityAssessmentData(final.Quality),
	}

	if final.SearchResult.Name != "" {
		data["provider"] = final.SearchResult.GetString("provider")
		data["results"] = final.SearchResult.Data["results"]
		data["resultCount"] = final.SearchResult.GetInt("resultCount")
		data["allowedCount"] = final.SearchResult.GetInt("allowedCount")
		data["neutralCount"] = final.SearchResult.GetInt("neutralCount")
		data["deniedCount"] = final.SearchResult.GetInt("deniedCount")
	}
	if final.FetchResult.Name != "" {
		data["pages"] = final.FetchResult.Data["pages"]
		data["combinedText"] = final.FetchResult.GetString("combinedText")
		data["successCount"] = final.FetchResult.GetInt("successCount")
		data["failCount"] = final.FetchResult.GetInt("failCount")
	}

	result := ragtool.Result{
		Name:    "external_evidence_workflow",
		Status:  ragtool.CallStatusSuccess,
		Summary: buildExternalEvidenceSummary(final),
		Data:    data,
	}
	log.Infof(
		"[external_evidence] workflow done: %s",
		ragtool.TruncateForLog(result.Summary),
	)
	return result, nil
}

func buildExternalEvidenceSummary(state *externalEvidenceState) string {
	searchView, _ := ragtool.ViewWebSearchResult(state.SearchResult)
	fetchView, _ := ragtool.ViewWebFetchResult(state.FetchResult)
	return fmt.Sprintf(
		"external evidence workflow: web_search=%s(%d results, allow=%d, neutral=%d, deny=%d) -> selected=%d -> web_fetch=%s(%d ok, %d failed) -> quality=%s(%.2f) -> readiness=%s(%.2f)",
		renderStatus(state.SearchResult.Status),
		searchView.ResultCount,
		searchView.AllowedCount,
		searchView.NeutralCount,
		searchView.DeniedCount,
		len(state.SelectedURLs),
		renderStatus(state.FetchResult.Status),
		fetchView.SuccessCount,
		fetchView.FailCount,
		state.Quality.Quality,
		state.Quality.Confidence,
		state.Readiness.Readiness,
		state.Readiness.Confidence,
	)
}

func buildExternalEvidenceStatus(state *externalEvidenceState) string {
	parts := make([]string, 0, 5)
	if state.SearchResult.Name != "" {
		parts = append(parts, fmt.Sprintf("web_search=%s", renderStatus(state.SearchResult.Status)))
	}
	if len(state.SelectedURLs) > 0 {
		parts = append(parts, fmt.Sprintf("selected=%d", len(state.SelectedURLs)))
	}
	if state.FetchResult.Name != "" {
		parts = append(parts, fmt.Sprintf("web_fetch=%s", renderStatus(state.FetchResult.Status)))
	}
	if state.Quality.Quality != "" {
		parts = append(parts, fmt.Sprintf("quality=%s", state.Quality.Quality))
	}
	if state.Readiness.Readiness != "" {
		parts = append(parts, fmt.Sprintf("readiness=%s", state.Readiness.Readiness))
	}
	return strings.Join(parts, " -> ")
}

func renderStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "skipped"
	}
	return status
}

func selectFetchURLs(view ragtool.WebSearchResultView, maxURLs int) []string {
	if len(view.Results) == 0 || maxURLs <= 0 {
		return nil
	}

	seenDomains := map[string]struct{}{}
	selected := make([]string, 0, maxURLs)
	appendItems := func(policy string) {
		for _, item := range view.Results {
			if len(selected) >= maxURLs {
				return
			}
			if strings.TrimSpace(item.URL) == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(item.Policy), "deny") {
				continue
			}
			itemPolicy := strings.TrimSpace(item.Policy)
			if !strings.EqualFold(itemPolicy, policy) {
				continue
			}
			domain := strings.TrimSpace(item.Domain)
			if domain == "" {
				domain = hostFromURL(item.URL)
			}
			if domain != "" {
				if _, exists := seenDomains[domain]; exists {
					continue
				}
			}
			selected = append(selected, strings.TrimSpace(item.URL))
			if domain != "" {
				seenDomains[domain] = struct{}{}
			}
		}
	}

	appendItems("allow")
	appendItems("neutral")
	return selected
}

func reviewExternalSources(view ragtool.WebSearchResultView, selectedURLs []string) externalSourceReview {
	review := externalSourceReview{
		TotalResults:    view.ResultCount,
		AllowedCount:    view.AllowedCount,
		NeutralCount:    view.NeutralCount,
		DeniedCount:     view.DeniedCount,
		SelectedCount:   len(selectedURLs),
		SelectedSources: make([]externalSourceSelection, 0, len(selectedURLs)),
		RejectedSources: make([]externalSourceSelection, 0, len(view.Results)),
	}
	selectedSet := make(map[string]struct{}, len(selectedURLs))
	selectedDomainSet := map[string]struct{}{}
	selectedSourceTypeSet := map[string]struct{}{}
	for _, rawURL := range selectedURLs {
		if trimmed := strings.TrimSpace(rawURL); trimmed != "" {
			selectedSet[trimmed] = struct{}{}
		}
	}

	for _, item := range view.Results {
		selection := externalSourceSelection{
			Title:         strings.TrimSpace(item.Title),
			URL:           strings.TrimSpace(item.URL),
			Domain:        strings.TrimSpace(item.Domain),
			Policy:        strings.TrimSpace(item.Policy),
			SourceType:    strings.TrimSpace(item.SourceType),
			ProviderScore: item.ProviderScore,
			RiskFlags:     uniqueTrimmedValues(item.RiskFlags),
			Reasons:       uniqueTrimmedValues(item.Reasons),
		}
		if selection.Domain == "" {
			selection.Domain = hostFromURL(selection.URL)
		}
		if _, ok := selectedSet[selection.URL]; ok {
			if selection.Domain != "" {
				selectedDomainSet[selection.Domain] = struct{}{}
			}
			if selection.SourceType != "" {
				selectedSourceTypeSet[selection.SourceType] = struct{}{}
			}
			review.SelectedSources = append(review.SelectedSources, selection)
			continue
		}
		if selection.URL != "" {
			review.RejectedSources = append(review.RejectedSources, selection)
		}
	}

	review.DistinctDomains = len(selectedDomainSet)
	review.DistinctSourceTypes = len(selectedSourceTypeSet)
	review.Coverage = deriveSourceCoverage(review)
	review.Notes = buildSourceReviewNotes(review)
	return review
}

func hostFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	return strings.TrimPrefix(host, "www.")
}

func assessExternalReadiness(ctx context.Context, state *externalEvidenceState, chatService aichat.LLMService) externalReadinessAssessment {
	availableURLs := collectAvailableEvidenceURLs(state)
	if chatService != nil {
		if assessment, ok := assessExternalReadinessWithLLM(ctx, state, chatService); ok {
			return normalizeReadinessAssessment(assessment, availableURLs)
		}
	}
	return normalizeReadinessAssessment(assessExternalReadinessFallback(state), availableURLs)
}

func assessExternalEvidenceQuality(state *externalEvidenceState) externalEvidenceQualityAssessment {
	fetchView, _ := ragtool.ViewWebFetchResult(state.FetchResult)
	quality := externalEvidenceQualityAssessment{
		SuccessfulPages: fetchView.SuccessCount,
		FailedPages:     fetchView.FailCount,
	}

	pagesWithText := 0
	for _, page := range fetchView.Pages {
		if strings.TrimSpace(page.Text) == "" {
			quality.EmptyPages++
		} else {
			pagesWithText++
		}
		if page.WasTruncated {
			quality.TruncatedPages++
		}
	}

	switch {
	case state.SourceReview.DistinctDomains >= 3 && state.SourceReview.DistinctSourceTypes >= 2:
		quality.SourceDiversity = "high"
	case state.SourceReview.DistinctDomains >= 2:
		quality.SourceDiversity = "medium"
	case state.SourceReview.SelectedCount >= 1:
		quality.SourceDiversity = "low"
	default:
		quality.SourceDiversity = "none"
	}

	switch {
	case pagesWithText >= 2:
		quality.Corroboration = "corroborated"
	case pagesWithText == 1:
		quality.Corroboration = "single_source"
	default:
		quality.Corroboration = "snippet_only"
	}

	switch {
	case pagesWithText == 0:
		quality.Quality = "limited"
		quality.Confidence = 0.38
		quality.Reasoning = "Search results were found, but no readable external page content was extracted."
		quality.Notes = append(quality.Notes, "No readable fetched page content was available.")
	case state.SourceReview.Coverage == "mixed" || quality.TruncatedPages > 0 || quality.FailedPages > 0:
		quality.Quality = "usable"
		quality.Confidence = 0.64
		quality.Reasoning = "Readable external evidence was fetched, but source coverage is mixed or some selected pages were incomplete."
	default:
		quality.Quality = "strong"
		quality.Confidence = 0.8
		quality.Reasoning = "Readable external evidence was fetched from selected sources with enough quality to ground an answer."
	}

	if state.SourceReview.DeniedCount > 0 {
		quality.Notes = append(quality.Notes, fmt.Sprintf("%d search result(s) were excluded by source policy.", state.SourceReview.DeniedCount))
	}
	if quality.TruncatedPages > 0 {
		quality.Notes = append(quality.Notes, fmt.Sprintf("%d fetched page(s) were truncated to the text budget.", quality.TruncatedPages))
	}
	if quality.FailedPages > 0 {
		quality.Notes = append(quality.Notes, fmt.Sprintf("%d selected source(s) failed during fetch.", quality.FailedPages))
	}
	quality.Notes = uniqueTrimmedValues(quality.Notes)
	return quality
}

func assessExternalReadinessWithLLM(ctx context.Context, state *externalEvidenceState, chatService aichat.LLMService) (externalReadinessAssessment, bool) {
	_ = ctx
	searchView, _ := ragtool.ViewWebSearchResult(state.SearchResult)
	fetchView, _ := ragtool.ViewWebFetchResult(state.FetchResult)

	var builder strings.Builder
	builder.WriteString("Assess whether the gathered external evidence is sufficient to answer the user's question.\n")
	builder.WriteString("Return strict JSON only with fields: readiness, confidence, reasoning, answerStrategy, missingInformation, citedUrls.\n")
	builder.WriteString("readiness must be one of: ready, partial, insufficient.\n")
	builder.WriteString("Prefer citedUrls that actually appear in the fetched or selected URLs. Do not invent URLs.\n\n")
	builder.WriteString("Question:\n")
	builder.WriteString(strings.TrimSpace(state.Question))
	builder.WriteString("\n\nSearch summary:\n")
	builder.WriteString(fmt.Sprintf(
		"provider=%s, results=%d, allow=%d, neutral=%d, deny=%d, selected=%d, coverage=%s, sourceDiversity=%s, corroboration=%s, quality=%s\n",
		searchView.Provider,
		searchView.ResultCount,
		searchView.AllowedCount,
		searchView.NeutralCount,
		searchView.DeniedCount,
		len(state.SelectedURLs),
		state.SourceReview.Coverage,
		state.Quality.SourceDiversity,
		state.Quality.Corroboration,
		state.Quality.Quality,
	))
	for idx, item := range searchView.Results {
		if idx >= 3 {
			break
		}
		builder.WriteString(fmt.Sprintf("%d. %s (%s) [policy=%s, type=%s]: %s\n", idx+1, item.Title, item.URL, item.Policy, item.SourceType, item.Snippet))
	}
	builder.WriteString("\nFetched content excerpt:\n")
	builder.WriteString(truncateForPrompt(fetchView.ReadableText(), 5000))
	if reasoning := strings.TrimSpace(state.Quality.Reasoning); reasoning != "" {
		builder.WriteString("\n\nCurrent quality assessment:\n")
		builder.WriteString(reasoning)
	}

	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage("You are judging answer readiness from external web evidence."),
			convention.UserMessage(builder.String()),
		},
	}
	jsonMode := true
	request.JSONMode = &jsonMode

	response, err := chatService.ChatWithRequest(request)
	if err != nil {
		return externalReadinessAssessment{}, false
	}
	var parsed externalReadinessAssessment
	if err := json.Unmarshal([]byte(strings.TrimSpace(response)), &parsed); err != nil {
		return externalReadinessAssessment{}, false
	}
	return parsed, true
}

func assessExternalReadinessFallback(state *externalEvidenceState) externalReadinessAssessment {
	searchView, _ := ragtool.ViewWebSearchResult(state.SearchResult)
	fetchView, _ := ragtool.ViewWebFetchResult(state.FetchResult)
	text := strings.TrimSpace(fetchView.ReadableText())

	switch {
	case searchView.ResultCount == 0:
		return externalReadinessAssessment{
			Readiness:      readinessInsufficient,
			Confidence:     0.2,
			Reasoning:      "External search returned no usable results, so there is not enough evidence to answer.",
			AnswerStrategy: "State that external evidence is unavailable and avoid making a definitive claim.",
		}
	case state.Quality.SuccessfulPages == 0 || text == "":
		return externalReadinessAssessment{
			Readiness:          readinessPartial,
			Confidence:         0.45,
			Reasoning:          "Search results exist, but no readable page content was fetched. Only snippet-level evidence is available.",
			AnswerStrategy:     "Answer cautiously using snippets only, explicitly mark the answer as low-confidence, and say what source details are still missing.",
			MissingInformation: []string{"Readable page content from external sources"},
		}
	case len(text) < 240 || state.Quality.Quality == "limited":
		return externalReadinessAssessment{
			Readiness:          readinessPartial,
			Confidence:         0.58,
			Reasoning:          "Readable page content was fetched, but the amount or quality of evidence is still limited.",
			AnswerStrategy:     "Answer with caveats, keep the conclusion narrow, and highlight what still needs verification.",
			MissingInformation: []string{"More corroborating detail from additional sources"},
			CitedURLs:          collectAvailableEvidenceURLs(state),
		}
	default:
		confidence := 0.74
		if state.Quality.Quality == "strong" {
			confidence = 0.82
		}
		if state.Quality.TruncatedPages > 0 || state.Quality.FailedPages > 0 {
			confidence = 0.66
		}
		return externalReadinessAssessment{
			Readiness:      readinessReady,
			Confidence:     confidence,
			Reasoning:      "Readable external evidence was fetched from selected sources, which is sufficient to answer with attribution.",
			AnswerStrategy: "Answer directly, cite the strongest sources first, and mention any remaining uncertainty after the main conclusion.",
			CitedURLs:      collectAvailableEvidenceURLs(state),
		}
	}
}

func normalizeReadinessAssessment(assessment externalReadinessAssessment, availableURLs []string) externalReadinessAssessment {
	assessment.Readiness = normalizeReadiness(assessment.Readiness)
	if assessment.Confidence < 0 {
		assessment.Confidence = 0
	}
	if assessment.Confidence > 1 {
		assessment.Confidence = 1
	}
	assessment.Reasoning = strings.TrimSpace(assessment.Reasoning)
	assessment.AnswerStrategy = strings.TrimSpace(assessment.AnswerStrategy)
	assessment.MissingInformation = uniqueTrimmedValues(assessment.MissingInformation)
	assessment.CitedURLs = filterKnownURLs(uniqueTrimmedValues(assessment.CitedURLs), availableURLs)
	if len(assessment.CitedURLs) == 0 {
		assessment.CitedURLs = filterKnownURLs(availableURLs, availableURLs)
	}
	return assessment
}

func normalizeReadiness(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case readinessReady:
		return readinessReady
	case readinessInsufficient:
		return readinessInsufficient
	default:
		return readinessPartial
	}
}

func collectAvailableEvidenceURLs(state *externalEvidenceState) []string {
	urls := make([]string, 0, len(state.SelectedURLs))
	seen := map[string]struct{}{}
	appendURL := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		urls = append(urls, raw)
	}
	for _, raw := range state.SelectedURLs {
		appendURL(raw)
	}
	fetchView, _ := ragtool.ViewWebFetchResult(state.FetchResult)
	for _, page := range fetchView.Pages {
		appendURL(page.URL)
	}
	return urls
}

func filterKnownURLs(candidates []string, known []string) []string {
	if len(candidates) == 0 {
		return nil
	}
	knownSet := make(map[string]struct{}, len(known))
	for _, item := range known {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			knownSet[trimmed] = struct{}{}
		}
	}
	filtered := make([]string, 0, len(candidates))
	for _, item := range candidates {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if len(knownSet) > 0 {
			if _, ok := knownSet[item]; !ok {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return uniqueTrimmedValues(filtered)
}

func buildSourceReviewData(review externalSourceReview) map[string]any {
	return map[string]any{
		"totalResults":        review.TotalResults,
		"allowedCount":        review.AllowedCount,
		"neutralCount":        review.NeutralCount,
		"deniedCount":         review.DeniedCount,
		"selectedCount":       review.SelectedCount,
		"distinctDomains":     review.DistinctDomains,
		"distinctSourceTypes": review.DistinctSourceTypes,
		"coverage":            review.Coverage,
		"notes":               review.Notes,
		"selectedSources":     buildSourceSelectionData(review.SelectedSources),
		"rejectedSources":     buildSourceSelectionData(review.RejectedSources),
	}
}

func buildSourceSelectionData(items []externalSourceSelection) []map[string]any {
	if len(items) == 0 {
		return []map[string]any{}
	}
	mapped := make([]map[string]any, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, map[string]any{
			"title":         item.Title,
			"url":           item.URL,
			"domain":        item.Domain,
			"policy":        item.Policy,
			"sourceType":    item.SourceType,
			"providerScore": item.ProviderScore,
			"riskFlags":     item.RiskFlags,
			"reasons":       item.Reasons,
		})
	}
	return mapped
}

func buildQualityAssessmentData(quality externalEvidenceQualityAssessment) map[string]any {
	return map[string]any{
		"quality":         quality.Quality,
		"confidence":      quality.Confidence,
		"reasoning":       quality.Reasoning,
		"sourceDiversity": quality.SourceDiversity,
		"corroboration":   quality.Corroboration,
		"successfulPages": quality.SuccessfulPages,
		"failedPages":     quality.FailedPages,
		"emptyPages":      quality.EmptyPages,
		"truncatedPages":  quality.TruncatedPages,
		"notes":           quality.Notes,
	}
}

func deriveSourceCoverage(review externalSourceReview) string {
	allowedSelected := 0
	neutralSelected := 0
	for _, item := range review.SelectedSources {
		switch strings.ToLower(strings.TrimSpace(item.Policy)) {
		case "allow":
			allowedSelected++
		case "neutral":
			neutralSelected++
		}
	}
	switch {
	case review.SelectedCount == 0:
		return "none"
	case allowedSelected > 0 && neutralSelected == 0:
		return "allow_only"
	case neutralSelected > 0 && allowedSelected == 0:
		return "neutral_only"
	default:
		return "mixed"
	}
}

func buildSourceReviewNotes(review externalSourceReview) []string {
	notes := make([]string, 0, 4)
	switch review.Coverage {
	case "allow_only":
		notes = append(notes, "Selected URLs all came from allow-listed or explicitly preferred sources.")
	case "neutral_only":
		notes = append(notes, "No allow-listed source was available, so the workflow relied on neutral sources.")
	case "mixed":
		notes = append(notes, "The workflow combined allow-listed and neutral sources to balance reliability and coverage.")
	}
	if review.DistinctDomains > 1 {
		notes = append(notes, fmt.Sprintf("Selected evidence spans %d distinct domains.", review.DistinctDomains))
	}
	if review.DistinctSourceTypes > 1 {
		notes = append(notes, fmt.Sprintf("Selected evidence covers %d source types.", review.DistinctSourceTypes))
	}
	return uniqueTrimmedValues(notes)
}

func extractSourceDomains(items []externalSourceSelection) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		if value := strings.TrimSpace(item.Domain); value != "" {
			values = append(values, value)
		}
	}
	return uniqueTrimmedValues(values)
}

func extractSourceTypes(items []externalSourceSelection) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		if value := strings.TrimSpace(item.SourceType); value != "" {
			values = append(values, value)
		}
	}
	return uniqueTrimmedValues(values)
}

func truncateForPrompt(text string, maxLen int) string {
	text = strings.TrimSpace(text)
	if len(text) <= maxLen {
		return text
	}
	return strings.TrimSpace(text[:maxLen-3]) + "..."
}

func uniqueTrimmedValues(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	values := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		values = append(values, item)
	}
	return values
}

func summarizeURLs(urls []string) string {
	if len(urls) == 0 {
		return "[]"
	}
	items := make([]string, 0, len(urls))
	for _, rawURL := range urls {
		if trimmed := strings.TrimSpace(rawURL); trimmed != "" {
			items = append(items, ragtool.TruncateForLog(trimmed))
		}
	}
	if len(items) == 0 {
		return "[]"
	}
	return "[" + strings.Join(items, ", ") + "]"
}
