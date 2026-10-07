package rag

import (
	"fmt"
	"strconv"
	"strings"

	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	runtimeadapter "local/rag-project/internal/adapter/runtime"
	profileservice "local/rag-project/internal/app/rag/service/profile"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/runtime/websource"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/infra-ai/chat"
)

func buildConversationRuntime(buildCtx *buildContext, conversation conversationBundle, retrieve retrieveBundle, memory memoryBundle, repos repositoriesBundle) (conversationruntime.ConversationRuntime, error) {
	if buildCtx == nil || buildCtx.cfg == nil || buildCtx.aiRuntime == nil {
		return nil, fmt.Errorf("runtime build context is required")
	}
	provider, ok := buildCtx.cfg.AI.Providers[runtimeadapter.SiliconFlowProviderID]
	if !ok {
		return nil, fmt.Errorf("runtime provider %q is not configured", runtimeadapter.SiliconFlowProviderID)
	}
	client, err := siliconFlowNativeClient(buildCtx.aiRuntime.ChatClients)
	if err != nil {
		return nil, err
	}
	tools := capability.NewRegistry()
	episodes := postgresruntime.NewStore(buildCtx.db)
	episodeService := capability.NewEpisodeService(episodes, buildCtx.aiRuntime.Embedding, runtimeID)
	if err := tools.Register(capability.ArchiveConversationEpisode(episodeService)); err != nil {
		return nil, fmt.Errorf("register archive conversation episode tool: %w", err)
	}
	if err := tools.Register(capability.SearchConversationHistory(episodeService)); err != nil {
		return nil, fmt.Errorf("register search conversation history tool: %w", err)
	}
	if err := tools.Register(capability.RetrieveKnowledge(retrieve.retrieveService)); err != nil {
		return nil, fmt.Errorf("register retrieve knowledge tool: %w", err)
	}
	if err := tools.Register(capability.CoreMemoryAdd(memory.explicitMemoryService)); err != nil {
		return nil, fmt.Errorf("register core memory add tool: %w", err)
	}
	if err := tools.Register(capability.CoreMemoryUpdate(memory.explicitMemoryService)); err != nil {
		return nil, fmt.Errorf("register core memory update tool: %w", err)
	}
	if err := tools.Register(capability.CoreMemoryDelete(memory.explicitMemoryService)); err != nil {
		return nil, fmt.Errorf("register core memory delete tool: %w", err)
	}
	policyCfg := buildCtx.cfg.Rag.Search.WebSearch.SourcePolicy
	policy := websource.New(websource.Config{AllowDomains: policyCfg.AllowDomains, DenyDomains: policyCfg.DenyDomains, AllowSuffixes: policyCfg.AllowSuffixes, DenySuffixes: policyCfg.DenySuffixes})
	if err := tools.Register(capability.WebSearch(runtimeadapter.NewTavilySearch(buildCtx.cfg.Rag.Search.WebSearch.ApiKey, buildCtx.aiRuntime.HTTPClient), policy)); err != nil {
		return nil, fmt.Errorf("register web search tool: %w", err)
	}
	if err := tools.Register(capability.WebFetch(runtimeadapter.NewWebFetcher(buildCtx.aiRuntime.HTTPClient), policy)); err != nil {
		return nil, fmt.Errorf("register web fetch tool: %w", err)
	}
	scheduledTasks := runtimeadapter.NewScheduledTasks(storepkg.NewStore(buildCtx.db))
	if err := tools.Register(capability.CreateScheduledTask(scheduledTasks)); err != nil {
		return nil, fmt.Errorf("register create scheduled task tool: %w", err)
	}
	if err := tools.Register(capability.ListScheduledTasks(scheduledTasks)); err != nil {
		return nil, fmt.Errorf("register list scheduled tasks tool: %w", err)
	}
	if err := tools.Register(capability.PauseScheduledTask(scheduledTasks)); err != nil {
		return nil, fmt.Errorf("register pause scheduled task tool: %w", err)
	}
	return &conversationruntime.Runtime{
		Model: runtimeadapter.NewSiliconFlowDeepSeekV4Flash(client, provider, false),
		Sources: conversationruntime.NewContextSources(
			conversationruntime.ContextSource{Key: "core/instructions", Render: func(conversationruntime.SourceContext) string {
				return conversationSystemInstruction
			}},
			conversationruntime.ContextSource{Key: "core/conversation-history", Render: func(conversationruntime.SourceContext) string {
				return conversationHistoryInstruction
			}},
			conversationruntime.ContextSource{Key: "core/scheduled-tasks", Render: func(conversationruntime.SourceContext) string {
				return conversationScheduledTaskInstruction
			}},
			conversationruntime.Date(),
			conversationruntime.LocalTime(),
			conversationruntime.ContextSource{Key: "memory/core-preferences", Render: func(source conversationruntime.SourceContext) string {
				return source.CoreMemoryContext
			}},
			conversationruntime.ContextSource{Key: "memory/derived-profile", Render: func(source conversationruntime.SourceContext) string {
				return source.DerivedProfileContext
			}},
		),
		History:                     runtimeadapter.NewConversationHistory(repos.messageRepo, repos.summaryRepo),
		Tools:                       tools,
		Lifecycle:                   conversationruntime.NewLifecycle(episodes, runtimeID),
		TaskJournal:                 postgresruntime.NewTaskJournal(buildCtx.db),
		Episodes:                    episodes,
		ContextTokenBudget:          runtimeContextTokenBudget(buildCtx),
		CoreMemoryContextLoader:     memory.explicitMemoryService.LoadCoreMemoryContext,
		DerivedProfileContextLoader: profileservice.NewService(repos.userMemoryProfileRepo).LoadContext,
	}, nil
}

func runtimeContextTokenBudget(buildCtx *buildContext) int {
	if buildCtx == nil || buildCtx.cfg == nil || !buildCtx.cfg.Rag.Memory.ChatContext.Enabled {
		return 0
	}
	chatContext := buildCtx.cfg.Rag.Memory.ChatContext
	budget := chatContext.MaxPromptTokens - chatContext.FixedReserveTokens - chatContext.SafetyReserveTokens
	if budget < 1 {
		return 0
	}
	return budget
}

func siliconFlowNativeClient(clients []chat.ChatClient) (chat.NativeStreamClient, error) {
	for _, client := range clients {
		if client == nil || !strings.EqualFold(client.Provider(), runtimeadapter.SiliconFlowProviderID) {
			continue
		}
		native, ok := client.(chat.NativeStreamClient)
		if !ok {
			return nil, fmt.Errorf("runtime provider %q does not support native streaming", runtimeadapter.SiliconFlowProviderID)
		}
		return native, nil
	}
	return nil, fmt.Errorf("runtime provider %q client is not configured", runtimeadapter.SiliconFlowProviderID)
}

func runtimeID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(id, 10), nil
}
