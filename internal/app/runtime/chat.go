package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/runtime/capability"
)

type ConversationMessage struct {
	ID             string
	ConversationID string
	UserID         string
	Role           ModelRole
	Content        string
	Sources        []domain.MessageSource
}

// ConversationMessages is chat's narrow durable boundary. Runtime journal is
// intentionally separate and remains owned by Lifecycle.
type ConversationMessages interface {
	Create(context.Context, ConversationMessage) (ConversationMessage, error)
}

type Conversations interface {
	Ensure(context.Context, string, string, string) error
}

// KnowledgeBaseAccessResolver is the server-owned authorization boundary for
// runtime retrieval. A caller-provided selection may only narrow this set.
type KnowledgeBaseAccessResolver interface {
	AccessibleIDs(context.Context, string) ([]string, error)
}

type ChatInput struct {
	// admitted is server-owned and cannot be supplied by an HTTP caller.
	admitted              *admittedChat
	Work                  *capability.WorkScope
	AcceptedUserMessageID string
	TaskID                string
	ConversationID        string
	UserID                string
	Question              string
	DeepThinking          bool
	KnowledgeBaseIDs      []string
	// Timezone is the browser-reported IANA zone that scheduled-task tools
	// resolve local times against. HTTP callers set it; the model never does.
	Timezone string
	Policy   Policy
	TraceID  string
}

type admittedChat struct {
	taskID, question string
	deepThinking     bool
	message          ConversationMessage
}

type ChatResult struct {
	UserMessage      ConversationMessage
	AssistantMessage ConversationMessage
	Runtime          RunResult
}

// ChatService makes the durable conversation boundary explicit: only accepted
// user input and terminal assistant content become conversation messages.
type ChatService struct {
	conversations           Conversations
	messages                ConversationMessages
	runtime                 ConversationRuntime
	tasks                   *TaskRegistry
	knowledgeBaseAccess     KnowledgeBaseAccessResolver
	afterAssistantPersisted func(context.Context, ConversationMessage)
	conversationGuard       func(context.Context, string, string) error
	taskAccess              func(context.Context, string, string) (bool, error)
	publication             ChatPublication
}

func NewChatService(conversations Conversations, messages ConversationMessages, runtime ConversationRuntime) *ChatService {
	return &ChatService{conversations: conversations, messages: messages, runtime: runtime, tasks: NewTaskRegistry()}
}

func (s *ChatService) SetKnowledgeBaseAccessResolver(resolver KnowledgeBaseAccessResolver) {
	if s != nil {
		s.knowledgeBaseAccess = resolver
	}
}

func (s *ChatService) SetAfterAssistantPersisted(hook func(context.Context, ConversationMessage)) {
	if s != nil {
		s.afterAssistantPersisted = hook
	}
}

func (s *ChatService) SetConversationGuard(guard func(context.Context, string, string) error) {
	if s != nil {
		s.conversationGuard = guard
	}
}

func (s *ChatService) SetTaskAccessResolver(access func(context.Context, string, string) (bool, error)) {
	if s != nil {
		s.taskAccess = access
	}
}

func (s *ChatService) AuthorizeTask(ctx context.Context, userID, taskID string) (bool, error) {
	if s == nil || s.taskAccess == nil {
		return false, fmt.Errorf("chat task access resolver is not configured")
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(taskID) == "" {
		return false, nil
	}
	return s.taskAccess(ctx, userID, taskID)
}

// Admit persists the user input and execution identity before the transport
// exposes its stream ID. Run reuses this session without invoking the model here.
func (s *ChatService) Admit(ctx context.Context, input ChatInput) (ChatInput, error) {
	if s == nil || s.conversations == nil || s.messages == nil || s.runtime == nil {
		return ChatInput{}, fmt.Errorf("runtime chat service is not configured")
	}
	if input.Work != nil || input.AcceptedUserMessageID != "" || input.admitted != nil || strings.TrimSpace(input.TaskID) == "" || input.TaskID != input.TraceID || strings.TrimSpace(input.ConversationID) == "" || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.Question) == "" {
		return ChatInput{}, fmt.Errorf("ordinary chat admission requires a new valid input")
	}
	admission, ok := s.runtime.(interface {
		Admit(context.Context, RunRequest) error
	})
	if !ok {
		return ChatInput{}, fmt.Errorf("runtime does not support durable chat admission")
	}
	if s.conversationGuard != nil {
		if err := s.conversationGuard(ctx, input.UserID, input.ConversationID); err != nil {
			return ChatInput{}, err
		}
	}
	if err := s.conversations.Ensure(ctx, input.ConversationID, input.UserID, input.Question); err != nil {
		return ChatInput{}, err
	}
	user, err := s.messages.Create(ctx, ConversationMessage{ConversationID: input.ConversationID, UserID: input.UserID, Role: ModelRoleUser, Content: input.Question})
	if err != nil {
		return ChatInput{}, err
	}
	if err := admission.Admit(ctx, RunRequest{publishAnswer: s.publication != nil, ConversationID: input.ConversationID, UserID: input.UserID, UserMessageID: user.ID, Question: input.Question, DeepThinking: input.DeepThinking, Timezone: input.Timezone, TraceID: input.TraceID}); err != nil {
		return ChatInput{}, err
	}
	input.admitted = &admittedChat{taskID: input.TaskID, question: input.Question, deepThinking: input.DeepThinking, message: user}
	return input, nil
}

func (s *ChatService) Chat(ctx context.Context, input ChatInput, sink EventSink) (ChatResult, error) {
	if s == nil || s.conversations == nil || s.messages == nil || s.runtime == nil {
		return ChatResult{}, fmt.Errorf("runtime chat service is not configured")
	}
	if strings.TrimSpace(input.TaskID) == "" || strings.TrimSpace(input.ConversationID) == "" || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.Question) == "" || strings.TrimSpace(input.TraceID) == "" {
		return ChatResult{}, fmt.Errorf("task id, conversation id, user id, question, and trace id are required")
	}
	if s.conversationGuard != nil {
		if err := s.conversationGuard(ctx, input.UserID, input.ConversationID); err != nil {
			return ChatResult{}, err
		}
	}
	taskCtx, done, err := s.tasks.Start(input.TaskID, ctx)
	if err != nil {
		return ChatResult{}, err
	}
	defer done()
	if s.knowledgeBaseAccess != nil {
		allowed, err := s.knowledgeBaseAccess.AccessibleIDs(taskCtx, input.UserID)
		if err != nil {
			return ChatResult{}, fmt.Errorf("resolve knowledge base access: %w", err)
		}
		if input.Work == nil || len(input.KnowledgeBaseIDs) > 0 {
			input.KnowledgeBaseIDs = selectKnowledgeBases(allowed, input.KnowledgeBaseIDs)
		}
	}
	user := ConversationMessage{ID: input.AcceptedUserMessageID, ConversationID: input.ConversationID, UserID: input.UserID, Role: ModelRoleUser, Content: input.Question}
	if input.admitted != nil {
		user = input.admitted.message
		if input.Work != nil || input.AcceptedUserMessageID != "" || user.UserID != input.UserID || user.ConversationID != input.ConversationID || input.admitted.question != input.Question || input.admitted.deepThinking != input.DeepThinking || input.TaskID != input.TraceID || input.TaskID != input.admitted.taskID {
			return ChatResult{}, fmt.Errorf("admitted chat identity cannot change")
		}
		allowed, accessErr := s.AuthorizeTask(taskCtx, input.UserID, input.TaskID)
		if accessErr != nil || !allowed {
			return ChatResult{}, fmt.Errorf("admitted chat is no longer accessible: %v", accessErr)
		}
	} else {
		if err := s.conversations.Ensure(taskCtx, input.ConversationID, input.UserID, input.Question); err != nil {
			return ChatResult{}, fmt.Errorf("ensure conversation: %w", err)
		}
	}
	if user.ID == "" {
		user, err = s.messages.Create(taskCtx, user)
	} else if input.Work == nil && input.admitted == nil {
		return ChatResult{}, fmt.Errorf("preaccepted messages require a Work scope")
	}
	if err != nil {
		return ChatResult{}, fmt.Errorf("persist user message: %w", err)
	}
	if strings.TrimSpace(user.ID) == "" {
		return ChatResult{}, fmt.Errorf("persisted user message has no id")
	}
	result, err := s.runtime.Run(taskCtx, RunRequest{
		publishAnswer:  s.publication != nil,
		DeepThinking:   input.DeepThinking,
		Work:           input.Work,
		ConversationID: input.ConversationID, UserID: input.UserID, UserMessageID: user.ID, Question: input.Question,
		KnowledgeBaseIDs: append([]string(nil), input.KnowledgeBaseIDs...), Timezone: input.Timezone, Policy: input.Policy, TraceID: input.TraceID,
	}, sink)
	if err != nil {
		return ChatResult{UserMessage: user, Runtime: result}, err
	}
	if result.Status != StatusCompleted && result.Status != StatusDegraded {
		return ChatResult{UserMessage: user, Runtime: result}, fmt.Errorf("runtime ended with non-terminal-answer status %q", result.Status)
	}
	if s.publication != nil {
		assistant, finish, err := s.publication.Publish(context.WithoutCancel(taskCtx), result.RuntimeSessionID)
		out := ChatResult{UserMessage: user, AssistantMessage: assistant, Runtime: result}
		if err != nil {
			return out, &PublicationPendingError{Cause: err}
		}
		if s.afterAssistantPersisted != nil {
			s.afterAssistantPersisted(context.WithoutCancel(taskCtx), assistant)
		}
		if sink != nil {
			if err = sink.Append(context.WithoutCancel(taskCtx), finish); err != nil {
				err = &PublicationPendingError{Cause: err}
			}
		}
		return out, err
	}
	assistant, err := s.messages.Create(taskCtx, ConversationMessage{ConversationID: input.ConversationID, UserID: input.UserID, Role: ModelRoleAssistant, Content: result.AssistantContent})
	if err != nil {
		if finalizer, ok := s.runtime.(ConversationEpisodeFinalizer); ok {
			_ = finalizer.DiscardEpisodes(context.WithoutCancel(taskCtx), result.RuntimeSessionID)
		}
		return ChatResult{UserMessage: user, Runtime: result}, fmt.Errorf("persist assistant message: %w", err)
	}
	if finalizer, ok := s.runtime.(ConversationEpisodeFinalizer); ok {
		if err := finalizer.CompleteEpisodes(context.WithoutCancel(taskCtx), result.RuntimeSessionID, assistant.ID); err != nil {
			return ChatResult{UserMessage: user, AssistantMessage: assistant, Runtime: result}, fmt.Errorf("complete conversation episodes: %w", err)
		}
	}
	if s.afterAssistantPersisted != nil {
		s.afterAssistantPersisted(context.WithoutCancel(taskCtx), assistant)
	}
	if sink != nil {
		finish, _ := json.Marshal(struct {
			MessageID string                 `json:"messageId"`
			Content   string                 `json:"content"`
			Sources   []domain.MessageSource `json:"sources"`
		}{assistant.ID, assistant.Content, assistant.Sources})
		if err := sink.Append(context.WithoutCancel(taskCtx), JournalEntry{RuntimeSessionID: result.RuntimeSessionID, ConversationID: input.ConversationID, UserMessageID: user.ID, TraceID: input.TraceID, EventType: EventCompleted, Detail: string(finish)}); err != nil {
			return ChatResult{UserMessage: user, AssistantMessage: assistant, Runtime: result}, err
		}
	}
	return ChatResult{UserMessage: user, AssistantMessage: assistant, Runtime: result}, nil
}

func selectKnowledgeBases(allowed, selected []string) []string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, id := range allowed {
		if id = strings.TrimSpace(id); id != "" {
			allowedSet[id] = struct{}{}
		}
	}
	if len(selected) == 0 {
		return uniqueTrimmed(allowed)
	}
	result := make([]string, 0, len(selected))
	for _, id := range uniqueTrimmed(selected) {
		if _, ok := allowedSet[id]; ok {
			result = append(result, id)
		}
	}
	return result
}

func (s *ChatService) CancelTask(taskID string) bool {
	return s != nil && s.tasks.Cancel(taskID)
}

// Replay delegates reconnect reads to the configured runtime. Chat keeps the
// transport-facing boundary while Runtime remains the owner of its journal.
func (s *ChatService) Replay(ctx context.Context, userID, taskID string, sink EventSink) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("runtime chat service is not configured")
	}
	if s.publication != nil {
		finish, err := s.publication.Recover(ctx, userID, taskID)
		if err != nil {
			return false, err
		}
		if finish.ID != "" {
			sink = publicationReplaySink{sink}
		}
	}
	replay, ok := s.runtime.(ConversationReplay)
	if !ok {
		return false, fmt.Errorf("runtime does not support replay")
	}
	return replay.Replay(ctx, userID, taskID, sink)
}
