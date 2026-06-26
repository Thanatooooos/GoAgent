package agent

import (
	"context"
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentcontentsummarize "local/rag-project/internal/app/agent/content_summarize"
	agentdocumentinvestigation "local/rag-project/internal/app/agent/document_investigation"
	agentexternal "local/rag-project/internal/app/agent/external_evidence"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agenthandoff "local/rag-project/internal/app/agent/handoff"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentknowledgediscovery "local/rag-project/internal/app/agent/knowledge_discovery"
	agentmemoryrecall "local/rag-project/internal/app/agent/memory_recall"
	agentpattern "local/rag-project/internal/app/agent/pattern"
	agentplanexecute "local/rag-project/internal/app/agent/pattern/planexecute"
	agentreactive "local/rag-project/internal/app/agent/pattern/reactive"
	agentplanner "local/rag-project/internal/app/agent/planner"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	agentstate "local/rag-project/internal/app/agent/state"
	agentthink "local/rag-project/internal/app/agent/think"
	"local/rag-project/internal/framework/config"
	aichat "local/rag-project/internal/infra-ai/chat"
)

const (
	PatternReactive    = "reactive"
	PatternPlanExecute = "plan_execute"
)

func defaultPattern() string {
	return PatternPlanExecute
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
	pendingStore := opts.PendingApprovalStore
	if pendingStore == nil {
		pendingStore = agentruntime.NewMemoryPendingApprovalStore()
	}
	outputMode := strings.TrimSpace(opts.OutputMode)
	if outputMode == "" {
		outputMode = agentstate.OutputModeHandoff
	}
	patternName := normalizePattern(opts.Pattern)
	runtimeName := runtimeNameForPattern(patternName)
	registry, bindings, err := assembleCapabilities(
		searchService,
		fetchService,
		opts.DocumentInvestigator,
		opts.LLMService,
		opts.KnowledgeDiscoverer,
		opts.MemoryRecaller,
	)
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
		kernelRunner:  runner,
		runtimeEngine: agentruntime.NewEngine(runner),
		handoff:       handoffBuilder,
		registry:      registry,
		bindings:      bindings,
		sessionStore:  sessionStore,
		pendingStore:  pendingStore,
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

func assembleCapabilities(
	searchService *agentsearch.Service,
	fetchService *agentfetch.Service,
	documentInvestigator agentdocumentinvestigation.Investigator,
	llmService aichat.LLMService,
	knowledgeDiscoverer agentknowledgediscovery.KnowledgeDiscoverer,
	memoryRecaller agentmemoryrecall.MemoryRecaller,
) (*agentcapability.Registry, agentcapability.RoleBindings, error) {
	registry := agentcapability.NewRegistry()
	if err := registerExternalEvidenceCapabilities(registry, searchService, fetchService); err != nil {
		return nil, nil, err
	}
	if err := registerOptionalWorkflowCapabilities(registry, documentInvestigator); err != nil {
		return nil, nil, err
	}
	if err := registerMetaCapabilities(registry, llmService); err != nil {
		return nil, nil, err
	}
	if err := registerDiscoveryCapabilities(registry, knowledgeDiscoverer); err != nil {
		return nil, nil, err
	}
	if err := registerMemoryCapabilities(registry, memoryRecaller); err != nil {
		return nil, nil, err
	}

	bindings := agentcapability.RoleBindings{
		agentcapability.RoleSearch: agentcapability.NameWebSearch,
		agentcapability.RoleFetch:  agentcapability.NameWebFetch,
	}
	return registry, bindings, nil
}

func registerMetaCapabilities(registry *agentcapability.Registry, llmService aichat.LLMService) error {
	thinkCapability, err := agentthink.NewCapability()
	if err != nil {
		return fmt.Errorf("meta capability %q construction failed: %w", agentcapability.NameThink, err)
	}
	handles := []agentcapability.Handle{thinkCapability}
	if llmService != nil {
		summarizeCapability, err := agentcontentsummarize.NewCapability(llmService)
		if err != nil {
			return fmt.Errorf("meta capability %q construction failed: %w", agentcapability.NameContentSummarize, err)
		}
		handles = append(handles, summarizeCapability)
	}
	return registerCapabilityGroup(registry, "meta", handles...)
}

func registerDiscoveryCapabilities(registry *agentcapability.Registry, discoverer agentknowledgediscovery.KnowledgeDiscoverer) error {
	if discoverer == nil {
		return nil
	}
	discoveryCapability, err := agentknowledgediscovery.NewCapability(discoverer)
	if err != nil {
		return fmt.Errorf("discovery capability %q construction failed: %w", agentcapability.NameKnowledgeDiscovery, err)
	}
	return registerCapabilityGroup(registry, "discovery", discoveryCapability)
}

func registerMemoryCapabilities(registry *agentcapability.Registry, recaller agentmemoryrecall.MemoryRecaller) error {
	if recaller == nil {
		return nil
	}
	memoryCapability, err := agentmemoryrecall.NewCapability(recaller)
	if err != nil {
		return fmt.Errorf("memory capability %q construction failed: %w", agentcapability.NameMemoryRecall, err)
	}
	return registerCapabilityGroup(registry, "memory", memoryCapability)
}

func registerExternalEvidenceCapabilities(registry *agentcapability.Registry, searchService *agentsearch.Service, fetchService *agentfetch.Service) error {
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		return fmt.Errorf("external evidence capability %q construction failed: %w", agentcapability.NameWebSearch, err)
	}
	fetchCapability, err := agentfetch.NewCapability(fetchService)
	if err != nil {
		return fmt.Errorf("external evidence capability %q construction failed: %w", agentcapability.NameWebFetch, err)
	}
	externalEvidenceCapability, err := agentexternal.NewCapability(searchCapability, fetchCapability)
	if err != nil {
		return fmt.Errorf("external evidence capability %q construction failed: %w", agentcapability.NameExternalEvidenceCollect, err)
	}
	return registerCapabilityGroup(registry, "external evidence", searchCapability, fetchCapability, externalEvidenceCapability)
}

func registerOptionalWorkflowCapabilities(registry *agentcapability.Registry, documentInvestigator agentdocumentinvestigation.Investigator) error {
	if documentInvestigator == nil {
		return nil
	}
	documentInvestigationCapability, err := agentdocumentinvestigation.NewCapability(documentInvestigator)
	if err != nil {
		return fmt.Errorf("optional workflow capability %q construction failed: %w", agentcapability.NameDocumentInvestigation, err)
	}
	return registerCapabilityGroup(registry, "optional workflow", documentInvestigationCapability)
}

func registerCapabilityGroup(registry *agentcapability.Registry, group string, handles ...agentcapability.Handle) error {
	for _, handle := range handles {
		if handle == nil {
			continue
		}
		spec := handle.Spec()
		if err := registry.Register(handle); err != nil {
			return fmt.Errorf("%s capability %q registration failed: %w", strings.TrimSpace(group), strings.TrimSpace(spec.Name), err)
		}
	}
	return nil
}

func normalizePattern(pattern string) string {
	switch strings.TrimSpace(pattern) {
	case PatternPlanExecute:
		return PatternPlanExecute
	case PatternReactive:
		return PatternReactive
	case "":
		return defaultPattern()
	default:
		return defaultPattern()
	}
}

func runtimeNameForPattern(pattern string) string {
	switch normalizePattern(pattern) {
	case PatternPlanExecute:
		return "agent_service_plan_execute"
	default:
		return "agent_service_reactive"
	}
}
