package web

import (
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

type WebSearchItemView struct {
	Title         string
	URL           string
	Snippet       string
	Domain        string
	Provider      string
	SourceType    string
	Policy        string
	ProviderScore float64
	RiskFlags     []string
	Reasons       []string
}

type WebSearchResultView struct {
	Query        string
	Provider     string
	Results      []WebSearchItemView
	ResultCount  int
	AllowedCount int
	NeutralCount int
	DeniedCount  int
}

func ViewWebSearchResult(result ragcore.Result) (WebSearchResultView, bool) {
	if strings.TrimSpace(result.Name) != "web_search" {
		return WebSearchResultView{}, false
	}
	view := WebSearchResultView{
		Query:        result.GetString("query"),
		Provider:     result.GetString("provider"),
		ResultCount:  result.GetInt("resultCount"),
		AllowedCount: result.GetInt("allowedCount"),
		NeutralCount: result.GetInt("neutralCount"),
		DeniedCount:  result.GetInt("deniedCount"),
	}
	items := ragcore.ReadMapItems(result.Data["results"])
	for _, item := range items {
		entry := WebSearchItemView{
			Title:         strings.TrimSpace(ragcore.ReadDataString(item, "title")),
			URL:           strings.TrimSpace(ragcore.ReadDataString(item, "url")),
			Snippet:       strings.TrimSpace(ragcore.ReadDataString(item, "snippet")),
			Domain:        strings.TrimSpace(ragcore.ReadDataString(item, "domain")),
			Provider:      strings.TrimSpace(ragcore.ReadDataString(item, "provider")),
			SourceType:    strings.TrimSpace(ragcore.ReadDataString(item, "sourceType")),
			Policy:        strings.TrimSpace(ragcore.ReadDataString(item, "policy")),
			ProviderScore: ragcore.ReadDataFloat(item, "providerScore"),
			RiskFlags:     ragcore.ReadDataStringSlice(item, "riskFlags"),
			Reasons:       ragcore.ReadDataStringSlice(item, "reasons"),
		}
		if entry.Title == "" && entry.URL == "" && entry.Snippet == "" && entry.Domain == "" {
			continue
		}
		view.Results = append(view.Results, entry)
	}
	if view.ResultCount == 0 {
		view.ResultCount = len(view.Results)
	}
	return view, true
}

func (v WebSearchResultView) URLs(limit int) []string {
	return v.filteredURLs(limit, false)
}

func (v WebSearchResultView) FetchableURLs(limit int) []string {
	return v.filteredURLs(limit, true)
}

func (v WebSearchResultView) filteredURLs(limit int, skipDenied bool) []string {
	if len(v.Results) == 0 {
		return nil
	}
	urls := make([]string, 0, len(v.Results))
	for _, item := range v.Results {
		if item.URL == "" {
			continue
		}
		if skipDenied && strings.EqualFold(strings.TrimSpace(item.Policy), "deny") {
			continue
		}
		urls = append(urls, item.URL)
		if limit > 0 && len(urls) >= limit {
			break
		}
	}
	return urls
}

type WebFetchPageView struct {
	URL          string
	Text         string
	Error        string
	OriginalLen  int
	WasTruncated bool
}

type WebFetchResultView struct {
	URLs         []string
	Pages        []WebFetchPageView
	CombinedText string
	SuccessCount int
	FailCount    int
}

func ViewWebFetchResult(result ragcore.Result) (WebFetchResultView, bool) {
	if strings.TrimSpace(result.Name) != "web_fetch" {
		return WebFetchResultView{}, false
	}
	view := WebFetchResultView{
		URLs:         result.GetStringSlice("urls"),
		CombinedText: result.GetString("combinedText"),
		SuccessCount: result.GetInt("successCount"),
		FailCount:    result.GetInt("failCount"),
	}
	items := ragcore.ReadMapItems(result.Data["pages"])
	for _, item := range items {
		page := WebFetchPageView{
			URL:          strings.TrimSpace(ragcore.ReadDataString(item, "url")),
			Text:         strings.TrimSpace(ragcore.ReadDataString(item, "text")),
			Error:        strings.TrimSpace(ragcore.ReadDataString(item, "error")),
			OriginalLen:  ragcore.ReadDataInt(item, "originalLen"),
			WasTruncated: ragcore.ReadDataBool(item, "wasTruncated"),
		}
		if page.URL == "" && page.Text == "" && page.Error == "" {
			continue
		}
		view.Pages = append(view.Pages, page)
	}
	return view, true
}

func (v WebFetchResultView) ReadableText() string {
	if text := strings.TrimSpace(v.CombinedText); text != "" {
		return text
	}
	if len(v.Pages) == 0 {
		return ""
	}
	parts := make([]string, 0, len(v.Pages))
	for _, page := range v.Pages {
		if strings.TrimSpace(page.Text) == "" {
			continue
		}
		if page.URL != "" {
			parts = append(parts, "["+page.URL+"]\n"+page.Text)
		} else {
			parts = append(parts, page.Text)
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func (v WebFetchResultView) AnyPageTruncated() bool {
	for _, page := range v.Pages {
		if page.WasTruncated {
			return true
		}
	}
	return false
}

type ExternalEvidenceSourceItemView struct {
	Title         string
	URL           string
	Domain        string
	Policy        string
	SourceType    string
	ProviderScore float64
	RiskFlags     []string
	Reasons       []string
}

type ExternalEvidenceSourceReviewView struct {
	TotalResults        int
	AllowedCount        int
	NeutralCount        int
	DeniedCount         int
	SelectedCount       int
	DistinctDomains     int
	DistinctSourceTypes int
	Coverage            string
	Notes               []string
	SelectedSources     []ExternalEvidenceSourceItemView
	RejectedSources     []ExternalEvidenceSourceItemView
}

type ExternalEvidenceQualityView struct {
	Quality         string
	Confidence      float64
	Reasoning       string
	SourceDiversity string
	Corroboration   string
	SuccessfulPages int
	FailedPages     int
	EmptyPages      int
	TruncatedPages  int
	Notes           []string
}

type ExternalEvidenceWorkflowView struct {
	Question            string
	SearchQuery         string
	Provider            string
	Search              WebSearchResultView
	Fetch               WebFetchResultView
	SelectedURLs        []string
	SelectedDomains     []string
	SelectedSourceTypes []string
	SourceCoverage      string
	SourceReview        ExternalEvidenceSourceReviewView
	Quality             ExternalEvidenceQualityView
	Readiness           string
	ReadinessConfidence float64
	ReadinessReasoning  string
	AnswerStrategy      string
	MissingInformation  []string
	CitedURLs           []string
}

func ViewExternalEvidenceWorkflowResult(result ragcore.Result) (ExternalEvidenceWorkflowView, bool) {
	if strings.TrimSpace(result.Name) != "external_evidence_workflow" {
		return ExternalEvidenceWorkflowView{}, false
	}
	sourceReview := parseExternalEvidenceSourceReview(ragcore.ReadDataMap(result.Data, "sourceReview"))
	qualityView := parseExternalEvidenceQuality(ragcore.ReadDataMap(result.Data, "qualityAssessment"))
	searchView, _ := ViewWebSearchResult(ragcore.Result{Name: "web_search", Data: result.Data})
	fetchView, _ := ViewWebFetchResult(ragcore.Result{Name: "web_fetch", Data: result.Data})
	return ExternalEvidenceWorkflowView{
		Question:            result.GetString("question"),
		SearchQuery:         result.GetString("searchQuery"),
		Provider:            result.GetString("provider"),
		Search:              searchView,
		Fetch:               fetchView,
		SelectedURLs:        result.GetStringSlice("selectedUrls"),
		SelectedDomains:     result.GetStringSlice("selectedDomains"),
		SelectedSourceTypes: result.GetStringSlice("selectedSourceTypes"),
		SourceCoverage:      result.GetString("sourceCoverage"),
		SourceReview:        sourceReview,
		Quality:             qualityView,
		Readiness:           result.GetString("readiness"),
		ReadinessConfidence: ragcore.ReadDataFloat(result.Data, "readinessConfidence"),
		ReadinessReasoning:  result.GetString("readinessReasoning"),
		AnswerStrategy:      result.GetString("answerStrategy"),
		MissingInformation:  result.GetStringSlice("missingInformation"),
		CitedURLs:           result.GetStringSlice("citedUrls"),
	}, true
}

func parseExternalEvidenceSourceReview(data map[string]any) ExternalEvidenceSourceReviewView {
	if len(data) == 0 {
		return ExternalEvidenceSourceReviewView{}
	}
	return ExternalEvidenceSourceReviewView{
		TotalResults:        ragcore.ReadDataInt(data, "totalResults"),
		AllowedCount:        ragcore.ReadDataInt(data, "allowedCount"),
		NeutralCount:        ragcore.ReadDataInt(data, "neutralCount"),
		DeniedCount:         ragcore.ReadDataInt(data, "deniedCount"),
		SelectedCount:       ragcore.ReadDataInt(data, "selectedCount"),
		DistinctDomains:     ragcore.ReadDataInt(data, "distinctDomains"),
		DistinctSourceTypes: ragcore.ReadDataInt(data, "distinctSourceTypes"),
		Coverage:            ragcore.ReadDataString(data, "coverage"),
		Notes:               ragcore.ReadDataStringSlice(data, "notes"),
		SelectedSources:     parseExternalEvidenceSourceItems(data["selectedSources"]),
		RejectedSources:     parseExternalEvidenceSourceItems(data["rejectedSources"]),
	}
}

func parseExternalEvidenceQuality(data map[string]any) ExternalEvidenceQualityView {
	if len(data) == 0 {
		return ExternalEvidenceQualityView{}
	}
	return ExternalEvidenceQualityView{
		Quality:         ragcore.ReadDataString(data, "quality"),
		Confidence:      ragcore.ReadDataFloat(data, "confidence"),
		Reasoning:       ragcore.ReadDataString(data, "reasoning"),
		SourceDiversity: ragcore.ReadDataString(data, "sourceDiversity"),
		Corroboration:   ragcore.ReadDataString(data, "corroboration"),
		SuccessfulPages: ragcore.ReadDataInt(data, "successfulPages"),
		FailedPages:     ragcore.ReadDataInt(data, "failedPages"),
		EmptyPages:      ragcore.ReadDataInt(data, "emptyPages"),
		TruncatedPages:  ragcore.ReadDataInt(data, "truncatedPages"),
		Notes:           ragcore.ReadDataStringSlice(data, "notes"),
	}
}

func parseExternalEvidenceSourceItems(raw any) []ExternalEvidenceSourceItemView {
	items := ragcore.ReadMapItems(raw)
	if len(items) == 0 {
		return nil
	}
	views := make([]ExternalEvidenceSourceItemView, 0, len(items))
	for _, item := range items {
		entry := ExternalEvidenceSourceItemView{
			Title:         ragcore.ReadDataString(item, "title"),
			URL:           ragcore.ReadDataString(item, "url"),
			Domain:        ragcore.ReadDataString(item, "domain"),
			Policy:        ragcore.ReadDataString(item, "policy"),
			SourceType:    ragcore.ReadDataString(item, "sourceType"),
			ProviderScore: ragcore.ReadDataFloat(item, "providerScore"),
			RiskFlags:     ragcore.ReadDataStringSlice(item, "riskFlags"),
			Reasons:       ragcore.ReadDataStringSlice(item, "reasons"),
		}
		if entry.Title == "" && entry.URL == "" && entry.Domain == "" {
			continue
		}
		views = append(views, entry)
	}
	return views
}
