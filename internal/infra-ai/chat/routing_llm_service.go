package chat

import (
	"fmt"
	"slices"
	"time"

	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/infra-ai/enum"
	"local/rag-project/internal/infra-ai/model"
)

const firstPacketTimeout = 60 * time.Second

type RoutingLLmService struct {
	selector          *model.ModelSelector
	healthStore       *model.ModelHealthStore
	executor          *model.ModelRoutingExecutor
	clientsByProvider map[string]ChatClient
}

func NewRoutingLLmService(
	selector *model.ModelSelector,
	healthStore *model.ModelHealthStore,
	executor *model.ModelRoutingExecutor,
	clients []ChatClient,
) *RoutingLLmService {
	clientsByProvider := make(map[string]ChatClient, len(clients))
	for _, client := range clients {
		if client == nil {
			continue
		}
		clientsByProvider[client.Provider()] = client
	}

	return &RoutingLLmService{
		selector:          selector,
		healthStore:       healthStore,
		executor:          executor,
		clientsByProvider: clientsByProvider,
	}
}

func (r *RoutingLLmService) Chat(prompt string) (string, error) {
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.UserMessage(prompt),
		},
	}
	return r.ChatWithRequest(request)
}

func (r *RoutingLLmService) ChatWithRequest(request convention.ChatRequest) (string, error) {
	content, _, err := r.ChatWithRequestUsage(request)
	return content, err
}

func (r *RoutingLLmService) ChatWithRequestUsage(request convention.ChatRequest) (string, TokenUsage, error) {
	if r == nil || r.selector == nil {
		return "", TokenUsage{}, fmt.Errorf(errRoutingServiceNil)
	}

	type chatResult struct {
		content string
		usage   TokenUsage
	}

	result, err := model.ExecuteWithFallback(
		r.executor,
		enum.ModelCapabilityChat,
		r.selector.SelectChatCandidates(request.ThinkingEnabled()),
		r.resolveClient,
		func(client ChatClient, target model.ModelTarget) (chatResult, error) {
			if usageClient, ok := client.(UsageAwareChatClient); ok {
				content, usage, err := usageClient.ChatWithUsage(request, target)
				return chatResult{content: content, usage: usage}, err
			}
			content, err := client.Chat(request, target)
			return chatResult{content: content}, err
		},
	)
	if err != nil {
		return "", TokenUsage{}, err
	}
	return result.content, result.usage, nil
}

func (r *RoutingLLmService) ChatWithModel(request convention.ChatRequest, modelID string) (string, error) {
	if r == nil || r.selector == nil {
		return "", fmt.Errorf(errRoutingServiceNil)
	}

	if modelID == "" {
		return r.ChatWithRequest(request)
	}

	target, err := r.resolveTarget(modelID, request.ThinkingEnabled())
	if err != nil {
		return "", err
	}

	return model.ExecuteWithFallback(
		r.executor,
		enum.ModelCapabilityChat,
		[]model.ModelTarget{target},
		r.resolveClient,
		func(client ChatClient, target model.ModelTarget) (string, error) {
			return client.Chat(request, target)
		},
	)
}

func (r *RoutingLLmService) StreamChat(prompt string, callback StreamCallback) (StreamCancellationHandle, error) {
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.UserMessage(prompt),
		},
	}
	return r.StreamChatWithRequest(request, callback)
}

func (r *RoutingLLmService) StreamChatWithRequest(request convention.ChatRequest, callback StreamCallback) (StreamCancellationHandle, error) {
	if r == nil || r.selector == nil {
		return nil, fmt.Errorf(errRoutingServiceNil)
	}
	if callback == nil {
		return nil, fmt.Errorf(errCallbackNil)
	}

	targets := r.selector.SelectChatCandidates(request.ThinkingEnabled())
	if len(targets) == 0 {
		return nil, exception.NewRemoteException(errStreamNoProvider, nil)
	}

	healthStore := r.ensureHealthStore()
	var lastErr error
	for _, target := range targets {
		client, err := r.resolveClient(target)
		if err != nil {
			lastErr = err
			continue
		}
		if !healthStore.AllowCall(target.Id) {
			continue
		}

		bridge := NewProbeStreamBridge(callback)
		handle, err := client.StreamChat(request, bridge, target)
		if err != nil {
			healthStore.MarkFailure(target.Id)
			lastErr = err
			continue
		}
		if handle == nil {
			healthStore.MarkFailure(target.Id)
			lastErr = exception.NewRemoteException(errStreamStartFailed, nil)
			continue
		}

		result := bridge.AwaitFirstPacket(firstPacketTimeout)
		if result.IsSuccess() {
			healthStore.MarkSuccess(target.Id)
			return handle, nil
		}

		handle.Cancel()
		healthStore.MarkFailure(target.Id)
		lastErr = r.buildStreamResultError(result)
	}

	if lastErr == nil {
		lastErr = exception.NewRemoteException(errStreamAllFailed, nil)
	}
	callback.OnError(lastErr)
	return nil, lastErr
}

func (r *RoutingLLmService) resolveClient(target model.ModelTarget) (ChatClient, error) {
	client := r.clientsByProvider[target.Candidate.Provider]
	if client == nil {
		return nil, fmt.Errorf(errChatProviderMissingFmt, target.Candidate.Provider, target.Id)
	}
	return client, nil
}

func (r *RoutingLLmService) resolveTarget(modelID string, thinking bool) (model.ModelTarget, error) {
	targets := r.selector.SelectChatCandidates(thinking)
	index := slices.IndexFunc(targets, func(target model.ModelTarget) bool {
		return target.Id == modelID
	})
	if index < 0 {
		return model.ModelTarget{}, exception.NewRemoteException(fmt.Sprintf(errChatModelUnavailable, modelID), nil)
	}
	return targets[index], nil
}

func (r *RoutingLLmService) ensureHealthStore() *model.ModelHealthStore {
	if r.healthStore == nil {
		r.healthStore = model.NewModelHealthStore()
	}
	return r.healthStore
}

func (r *RoutingLLmService) buildStreamResultError(result ProbeResult) error {
	switch result.Type {
	case ProbeResultError:
		if result.Error != nil {
			return result.Error
		}
		return exception.NewRemoteException(errStreamStartFailed, nil)
	case ProbeResultTimeout:
		return exception.NewRemoteException(errStreamTimeout, nil)
	case ProbeResultNoContent:
		return exception.NewRemoteException(errStreamNoContent, nil)
	default:
		return exception.NewRemoteException(errStreamAllFailed, nil)
	}
}
