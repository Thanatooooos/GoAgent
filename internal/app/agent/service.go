package agent

import (
	"context"
	"net/http"
	"strings"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentdocumentinvestigation "local/rag-project/internal/app/agent/document_investigation"
	agentexternal "local/rag-project/internal/app/agent/external_evidence"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agenthandoff "local/rag-project/internal/app/agent/handoff"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentpattern "local/rag-project/internal/app/agent/pattern"
	agentplanexecute "local/rag-project/internal/app/agent/pattern/planexecute"
	agentreactive "local/rag-project/internal/app/agent/pattern/reactive"
	agentplanner "local/rag-project/internal/app/agent/planner"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	agentstate "local/rag-project/internal/app/agent/state"
	"local/rag-project/internal/framework/config"
	aichat "local/rag-project/internal/infra-ai/chat"
	inframcp "local/rag-project/internal/infra-mcp"
)

type ServiceOptions struct {
	Config                   *config.Config
	Provider                 searchprovider.SearchProvider
	SourcePolicy             *searchprovider.SourcePolicyEngine
	HTTPClient               *http.Client
	FetchService             *agentfetch.Service
	MCPManager               inframcp.ToolClient
	LLMService               aichat.LLMService
	CheckpointStore          agentkernel.CheckpointStore
	SessionStore             agentruntime.SessionStore
	OutputMode               string
	MaxIterations            int
	Pattern                  string
	CapabilityCatalogBuilder agentcatalog.Builder
	CapabilitySelector       selectcapability.Selector
	CapabilityResolver       agentresolve.Resolver
	DocumentInvestigator     agentdocumentinvestigation.Investigator
}

type Service struct {
	runner        *agentkernel.Runner
	handoff       *agenthandoff.Builder
	registry      *agentcapability.Registry
	bindings      agentcapability.RoleBindings
	sessionStore  agentruntime.SessionStore
	reducer       agentstate.Reducer
	maxIterations int
	outputMode    string
	pattern       string
	runtimeName   string
}

func NewService(opts ServiceOptions) (*Service, error) {
	cfg := opts.Config
	if cfg == nil {
		cfg = config.Get()
	}

	provider := opts.Provider
	if provider == nil {
		provider = searchprovider.BuildProvider(cfg, opts.MCPManager)
	}
	policy := opts.SourcePolicy
	if policy == nil {
		policy = searchprovider.BuildSourcePolicy(cfg)
	}

	searchService := agentsearch.NewService(provider, policy)
	fetchService := opts.FetchService
	if fetchService == nil {
		fetchService = agentfetch.NewService(opts.HTTPClient)
	}
	planner := agentplanner.NewLLMPlanner(opts.LLMService)
	checkpointStore := opts.CheckpointStore
	if checkpointStore == nil {
		checkpointStore = agentkernel.NewMemoryCheckpointStore()
	}
	sessionStore := opts.SessionStore
	if sessionStore == nil {
		sessionStore = agentruntime.NewMemorySessionStore()
	}
	outputMode := strings.TrimSpace(opts.OutputMode)
	if outputMode == "" {
		outputMode = agentstate.OutputModeHandoff
	}
	patternName := normalizePattern(opts.Pattern)
	runtimeName := runtimeNameForPattern(patternName)
	registry, bindings, err := assembleCapabilities(searchService, fetchService, opts.DocumentInvestigator)
	if err != nil {
		return nil, err
	}
	catalogBuilder := opts.CapabilityCatalogBuilder
	if catalogBuilder == nil {
		catalogBuilder = agentcatalog.NewBuilder()
	}
	capabilitySelector := opts.CapabilitySelector
	if capabilitySelector == nil && opts.LLMService != nil {
		capabilitySelector = selectcapability.NewLLMSelector(opts.LLMService)
	}
	capabilityResolver := opts.CapabilityResolver
	if capabilityResolver == nil {
		capabilityResolver = agentresolve.NewRegistryResolver(registry)
	}
	handoffBuilder := buildHandoffBuilder(registry, bindings, patternName)

	runner, err := compileRunner(context.Background(), patternName, registry, bindings, agentpattern.RuntimeConfig{
		Planner:                  planner,
		CapabilityCatalogBuilder: catalogBuilder,
		CapabilitySelector:       capabilitySelector,
		CapabilityResolver:       capabilityResolver,
		OutputMode:               outputMode,
		ApprovalSessionStore:     sessionStore,
		Kernel: agentkernel.BuilderConfig{
			GraphName:       runtimeName,
			Reducer:         agentstate.DefaultReducer{},
			CheckpointStore: checkpointStore,
		},
	})
	if err != nil {
		return nil, err
	}

	service := &Service{
		runner:        runner,
		handoff:       handoffBuilder,
		registry:      registry,
		bindings:      bindings,
		sessionStore:  sessionStore,
		reducer:       agentstate.DefaultReducer{},
		maxIterations: opts.MaxIterations,
		outputMode:    outputMode,
		pattern:       patternName,
		runtimeName:   runtimeName,
	}
	logAgentServiceInitialized(service.pattern, service.runtimeName, service.maxIterations, service.outputMode)
	return service, nil
}

func compileRunner(ctx context.Context, patternName string, registry *agentcapability.Registry, bindings agentcapability.RoleBindings, runtimeCfg agentpattern.RuntimeConfig) (*agentkernel.Runner, error) {
	assembly := agentpattern.AssemblyContext{
		Registry: registry,
		Bindings: bindings,
	}
	switch normalizePattern(patternName) {
	case PatternPlanExecute:
		return agentplanexecute.Compile(ctx, agentplanexecute.Config{
			Assembly: assembly,
			Runtime:  runtimeCfg,
		})
	default:
		return agentreactive.Compile(ctx, agentreactive.Config{
			Assembly: assembly,
			Runtime:  runtimeCfg,
		})
	}
}

func buildHandoffBuilder(registry *agentcapability.Registry, bindings agentcapability.RoleBindings, patternName string) *agenthandoff.Builder {
	switch normalizePattern(patternName) {
	case PatternReactive:
		return agenthandoff.NewBuilderFromRegistry(registry, agentreactive.HandoffBindings(bindings))
	default:
		return agenthandoff.NewBuilderFromRegistry(registry, nil)
	}
}

func assembleCapabilities(searchService *agentsearch.Service, fetchService *agentfetch.Service, documentInvestigator agentdocumentinvestigation.Investigator) (*agentcapability.Registry, agentcapability.RoleBindings, error) {
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		return nil, nil, err
	}
	fetchCapability, err := agentfetch.NewCapability(fetchService)
	if err != nil {
		return nil, nil, err
	}
	externalEvidenceCapability, err := agentexternal.NewCapability(searchCapability, fetchCapability)
	if err != nil {
		return nil, nil, err
	}
	var documentInvestigationCapability agentcapability.Handle
	if documentInvestigator != nil {
		documentInvestigationCapability, err = agentdocumentinvestigation.NewCapability(documentInvestigator)
		if err != nil {
			return nil, nil, err
		}
	}

	registry := agentcapability.NewRegistry()
	handles := []agentcapability.Handle{searchCapability, fetchCapability, externalEvidenceCapability}
	if documentInvestigationCapability != nil {
		handles = append(handles, documentInvestigationCapability)
	}
	for _, handle := range handles {
		if err := registry.Register(handle); err != nil {
			return nil, nil, err
		}
	}

	bindings := agentcapability.RoleBindings{
		agentcapability.RoleSearch: agentcapability.NameWebSearch,
		agentcapability.RoleFetch:  agentcapability.NameWebFetch,
	}
	return registry, bindings, nil
}

func newRuntimeSession(req Request, maxIterations int, outputMode string, runtimeName string) *agentruntime.RuntimeSession {
	question := strings.TrimSpace(req.Question)
	userID := strings.TrimSpace(req.UserID)
	traceID := strings.TrimSpace(req.TraceID)
	now := time.Now()
	if maxIterations <= 0 {
		maxIterations = defaultAgentMaxIterations()
	}
	if strings.TrimSpace(outputMode) == "" {
		outputMode = agentstate.OutputModeHandoff
	}
	options := agentstate.RuntimeOptions{
		MaxIterations:   firstPositive(req.Options.MaxIterations, maxIterations),
		AllowWebSearch:  true,
		RequireApproval: req.Options.RequireApproval,
		OutputMode:      firstNonEmpty(req.Options.OutputMode, outputMode),
	}
	if options.MaxIterations <= 0 {
		options.MaxIterations = defaultAgentMaxIterations()
	}
	if strings.TrimSpace(options.OutputMode) == "" {
		options.OutputMode = agentstate.OutputModeHandoff
	}
	execution := agentstate.ExecutionState{
		MaxIterations: options.MaxIterations,
	}

	session := &agentruntime.RuntimeSession{
		SessionID: firstNonEmpty(traceID, question, now.Format(time.RFC3339Nano)),
		Request: agentruntime.RequestEnvelope{
			Question: question,
			UserID:   userID,
			TraceID:  traceID,
			Options:  options,
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question:       question,
				UserID:         userID,
				TraceID:        traceID,
				RuntimeOptions: options,
			},
			Execution: execution,
		},
		Metadata: agentruntime.SessionMetadata{
			CreatedAt:   now,
			UpdatedAt:   now,
			RuntimeName: firstNonEmpty(runtimeName, runtimeNameForPattern(PatternReactive)),
		},
	}
	seedRuntimeSessionFromToolStage(session, req.ToolStage)
	return session
}

func defaultAgentMaxIterations() int {
	return 2
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func responseFromSession(session *agentruntime.RuntimeSession) Response {
	if session == nil {
		return Response{}
	}
	results := toSearchResults(session.Snapshot.Context.SearchResults)
	pages := toPages(session.Snapshot.Context.FetchResults)
	combinedText := buildCombinedText(pages)
	summary := firstNonEmpty(
		session.Snapshot.Answer.Final,
		latestNote(session.Snapshot.Context.Notes),
		session.Snapshot.Approval.Reason,
		session.Snapshot.Evidence.SufficiencyReason,
	)
	provider := strings.TrimSpace(firstNonEmpty(
		session.Snapshot.Context.SearchProviderActual,
		session.Snapshot.Context.SearchProvider,
	))
	degradeReason := strings.TrimSpace(session.Snapshot.Answer.DegradeReason)

	return Response{
		Query:         firstNonEmpty(session.Snapshot.Context.SearchQuery, session.Request.Question, session.Snapshot.Request.Question),
		Results:       results,
		Pages:         pages,
		CombinedText:  combinedText,
		Summary:       summary,
		Provider:      provider,
		Degraded:      degradeReason != "",
		DegradeReason: degradeReason,
	}
}

func toSearchResults(refs []agentstate.SearchResultRef) []agentsearch.SearchResultItem {
	if len(refs) == 0 {
		return nil
	}
	results := make([]agentsearch.SearchResultItem, 0, len(refs))
	for _, ref := range refs {
		results = append(results, agentsearch.SearchResultItem{
			Title:      ref.Title,
			URL:        ref.URL,
			Snippet:    ref.Snippet,
			Domain:     ref.Domain,
			SourceType: ref.SourceType,
			Policy:     ref.Policy,
			RiskFlags:  append([]string(nil), ref.RiskFlags...),
			Reasons:    append([]string(nil), ref.Reasons...),
		})
	}
	return results
}

func toPages(refs []agentstate.FetchResultRef) []agentfetch.PageResult {
	if len(refs) == 0 {
		return nil
	}
	pages := make([]agentfetch.PageResult, 0, len(refs))
	for _, ref := range refs {
		pages = append(pages, agentfetch.PageResult{
			URL:            ref.URL,
			Text:           ref.Text,
			ErrorMessage:   ref.ErrorReason,
			OriginalLength: ref.OriginalLength,
			WasTruncated:   ref.WasTruncated,
		})
	}
	return pages
}

func buildCombinedText(pages []agentfetch.PageResult) string {
	if len(pages) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, page := range pages {
		if strings.TrimSpace(page.Text) == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n---\n\n")
		}
		builder.WriteString("[")
		builder.WriteString(page.URL)
		builder.WriteString("]\n")
		builder.WriteString(page.Text)
	}
	return builder.String()
}

func latestNote(notes []string) string {
	for i := len(notes) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(notes[i]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
