package agent

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentdocumentinvestigation "local/rag-project/internal/app/agent/document_investigation"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agenthandoff "local/rag-project/internal/app/agent/handoff"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentknowledgediscovery "local/rag-project/internal/app/agent/knowledge_discovery"
	agentmemoryrecall "local/rag-project/internal/app/agent/memory_recall"
	agentruntime "local/rag-project/internal/app/agent/runtime"
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
	PendingApprovalStore     agentruntime.PendingApprovalStore
	OutputMode               string
	MaxIterations            int
	Pattern                  string
	CapabilityCatalogBuilder agentcatalog.Builder
	CapabilitySelector       selectcapability.Selector
	CapabilityResolver       agentresolve.Resolver
	DocumentInvestigator     agentdocumentinvestigation.Investigator
	KnowledgeDiscoverer      agentknowledgediscovery.KnowledgeDiscoverer
	MemoryRecaller           agentmemoryrecall.MemoryRecaller
}

type Service struct {
	// kernelRunner is the compiled graph execution layer. Service owns request
	// mapping and outward responses, but not the graph execution mechanics.
	kernelRunner  *agentkernel.Runner
	runtimeEngine *agentruntime.Engine
	handoff       *agenthandoff.Builder
	registry      *agentcapability.Registry
	bindings      agentcapability.RoleBindings
	sessionStore  agentruntime.SessionStore
	pendingStore  agentruntime.PendingApprovalStore
	reducer       agentstate.Reducer
	maxIterations int
	outputMode    string
	pattern       string
	runtimeName   string
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
			SchemaVersion: agentstate.CurrentSnapshotVersion,
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
			RuntimeName: firstNonEmpty(runtimeName, runtimeNameForPattern(defaultPattern())),
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func seedRuntimeSessionFromToolStage(session *agentruntime.RuntimeSession, context *ToolStageContext) {
	if session == nil || context == nil {
		return
	}

	if conversationID := strings.TrimSpace(context.ConversationID); conversationID != "" {
		session.Request.ConversationID = conversationID
		session.Snapshot.Request.ConversationID = conversationID
	}
	if knowledgeBaseIDs := uniqueTrimmedStrings(context.KnowledgeBaseIDs); len(knowledgeBaseIDs) > 0 {
		session.Snapshot.Request.KnowledgeBaseIDs = knowledgeBaseIDs
	}

	rewrittenQuestion := strings.TrimSpace(context.RewrittenQuestion)
	if rewrittenQuestion != "" {
		session.Snapshot.Context.RewrittenQuery = rewrittenQuestion
	}
	session.Snapshot.Context.SearchQuery = firstNonEmpty(
		rewrittenQuestion,
		strings.TrimSpace(session.Request.Question),
		strings.TrimSpace(session.Snapshot.Request.Question),
	)

	notes := buildToolStageNotes(context)
	if len(notes) > 0 {
		session.Snapshot.Context.Notes = append([]string(nil), notes...)
	}
}

func buildToolStageNotes(context *ToolStageContext) []string {
	if context == nil {
		return nil
	}

	notes := make([]string, 0, 8)
	if len(context.SubQuestions) > 0 {
		notes = append(notes, "tool-stage sub-questions: "+strings.Join(uniqueTrimmedStrings(context.SubQuestions), " | "))
	}
	if context.NeedRetrieval {
		notes = append(notes, "tool-stage retrieval was requested before agent handoff")
	}
	if summary := summarizeToolStageText("tool-stage history summary", context.HistorySummary, 320); summary != "" {
		notes = append(notes, summary)
	}
	if summary := summarizeToolStageText("tool-stage session context", context.SessionContext, 320); summary != "" {
		notes = append(notes, summary)
	}
	if summary := summarizeToolStageText("tool-stage memory context", context.MemoryContext, 320); summary != "" {
		notes = append(notes, summary)
	}
	if summary := summarizeToolStageText("tool-stage knowledge context", context.KnowledgeContext, 400); summary != "" {
		notes = append(notes, summary)
	}
	if len(context.SearchChannels) > 0 {
		notes = append(notes, "tool-stage search channels: "+strings.Join(uniqueTrimmedStrings(context.SearchChannels), ", "))
	}
	return uniqueTrimmedStrings(notes)
}

func summarizeToolStageText(label string, value string, limit int) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if trimmed == "" {
		return ""
	}
	if limit > 0 && utf8.RuneCountInString(trimmed) > limit {
		trimmed = strings.TrimSpace(string([]rune(trimmed)[:limit-3])) + "..."
	}
	return label + ": " + trimmed
}

func uniqueTrimmedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
