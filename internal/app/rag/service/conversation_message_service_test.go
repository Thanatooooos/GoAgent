package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
)

type conversationMessageConversationRepoStub struct {
	getByConversationIDAndUserFn func(ctx context.Context, conversationID string, userID string) (domain.Conversation, error)
}

func (s conversationMessageConversationRepoStub) Create(context.Context, domain.Conversation) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}

func (s conversationMessageConversationRepoStub) Update(context.Context, domain.Conversation) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}

func (s conversationMessageConversationRepoStub) UpdateWhere(context.Context, port.ConversationConditions, port.ConversationPatch) (int64, error) {
	return 0, nil
}

func (s conversationMessageConversationRepoStub) Delete(context.Context, string) error {
	return nil
}

func (s conversationMessageConversationRepoStub) GetByID(context.Context, string) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}

func (s conversationMessageConversationRepoStub) GetByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) (domain.Conversation, error) {
	return s.getByConversationIDAndUserFn(ctx, conversationID, userID)
}

func (s conversationMessageConversationRepoStub) ListByUserID(context.Context, string) ([]domain.Conversation, error) {
	return nil, nil
}

type conversationMessageRepoServiceStub struct {
	createFn func(ctx context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error)
	listFn   func(ctx context.Context, filter port.ConversationMessageListFilter) ([]domain.ConversationMessage, error)
}

func (s conversationMessageRepoServiceStub) Create(ctx context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
	return s.createFn(ctx, message)
}

func (s conversationMessageRepoServiceStub) GetByID(context.Context, string) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, nil
}

func (s conversationMessageRepoServiceStub) List(ctx context.Context, filter port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
	return s.listFn(ctx, filter)
}

func (s conversationMessageRepoServiceStub) CountByConversationIDAndUserIDAndRole(context.Context, string, string, string) (int64, error) {
	return 0, nil
}

func (s conversationMessageRepoServiceStub) FindMaxIDAtOrBefore(context.Context, string, string, time.Time) (string, error) {
	return "", nil
}

func (s conversationMessageRepoServiceStub) DeleteByConversationIDAndUserID(context.Context, string, string) error {
	return nil
}

type conversationSummaryRepoServiceStub struct {
	createFn func(ctx context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error)
	latestFn func(ctx context.Context, conversationID string, userID string) (domain.ConversationSummary, error)
}

func (s conversationSummaryRepoServiceStub) Create(ctx context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error) {
	return s.createFn(ctx, summary)
}

func (s conversationSummaryRepoServiceStub) FindLatestByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) (domain.ConversationSummary, error) {
	if s.latestFn != nil {
		return s.latestFn(ctx, conversationID, userID)
	}
	return domain.ConversationSummary{}, nil
}

func (s conversationSummaryRepoServiceStub) DeleteByConversationIDAndUserID(context.Context, string, string) error {
	return nil
}

type messageFeedbackRepoServiceStub struct {
	listFn func(ctx context.Context, userID string, messageIDs []string) ([]domain.MessageFeedback, error)
}

func (s messageFeedbackRepoServiceStub) Create(context.Context, domain.MessageFeedback) (domain.MessageFeedback, error) {
	return domain.MessageFeedback{}, nil
}

func (s messageFeedbackRepoServiceStub) Update(context.Context, domain.MessageFeedback) (domain.MessageFeedback, error) {
	return domain.MessageFeedback{}, nil
}

func (s messageFeedbackRepoServiceStub) UpdateWhere(context.Context, port.MessageFeedbackConditions, port.MessageFeedbackPatch) (int64, error) {
	return 0, nil
}

func (s messageFeedbackRepoServiceStub) GetByMessageIDAndUserID(context.Context, string, string) (domain.MessageFeedback, error) {
	return domain.MessageFeedback{}, nil
}

func (s messageFeedbackRepoServiceStub) ListByUserIDAndMessageIDs(ctx context.Context, userID string, messageIDs []string) ([]domain.MessageFeedback, error) {
	return s.listFn(ctx, userID, messageIDs)
}

func TestConversationMessageServiceAddMessageCreatesRecord(t *testing.T) {
	var created domain.ConversationMessage
	service := NewConversationMessageService(
		conversationMessageConversationRepoStub{getByConversationIDAndUserFn: func(context.Context, string, string) (domain.Conversation, error) {
			return domain.Conversation{}, nil
		}},
		conversationMessageRepoServiceStub{
			createFn: func(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
				created = message
				return message, nil
			},
			listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
				return nil, nil
			},
		},
		conversationSummaryRepoServiceStub{createFn: func(context.Context, domain.ConversationSummary) (domain.ConversationSummary, error) {
			return domain.ConversationSummary{}, nil
		}},
		nil,
	)

	result, err := service.AddMessage(context.Background(), AddConversationMessageInput{
		ConversationID: "c1",
		UserID:         "u1",
		Role:           convention.UserRole,
		Content:        "你好",
	})
	if err != nil {
		t.Fatalf("AddMessage returned error: %v", err)
	}
	if result.ID == "" || created.ID == "" {
		t.Fatal("expected generated message id")
	}
	if created.Role != "user" || created.Content != "你好" {
		t.Fatalf("unexpected message: %#v", created)
	}
	if created.IsSummarized {
		t.Fatalf("expected unsummarized message, got %#v", created)
	}
}

func TestConversationMessageServiceAddMessageStoresSummaryFields(t *testing.T) {
	var created domain.ConversationMessage
	service := NewConversationMessageService(
		conversationMessageConversationRepoStub{getByConversationIDAndUserFn: func(context.Context, string, string) (domain.Conversation, error) {
			return domain.Conversation{}, nil
		}},
		conversationMessageRepoServiceStub{
			createFn: func(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
				created = message
				return message, nil
			},
			listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
				return nil, nil
			},
		},
		conversationSummaryRepoServiceStub{createFn: func(context.Context, domain.ConversationSummary) (domain.ConversationSummary, error) {
			return domain.ConversationSummary{}, nil
		}},
		nil,
	)

	_, err := service.AddMessage(context.Background(), AddConversationMessageInput{
		ConversationID: "c1",
		UserID:         "u1",
		Role:           convention.UserRole,
		Content:        "精简摘要",
		RawContent:     "很长很长的原文",
		ContentSummary: "精简摘要",
		IsSummarized:   true,
	})
	if err != nil {
		t.Fatalf("AddMessage returned error: %v", err)
	}
	if created.RawContent != "很长很长的原文" || created.ContentSummary != "精简摘要" || !created.IsSummarized {
		t.Fatalf("unexpected summary fields: %#v", created)
	}
}

type summaryProcessorStub struct {
	result ProcessedConversationMessageContent
	err    error
}

func (s summaryProcessorStub) ProcessAddMessage(context.Context, AddConversationMessageInput) (ProcessedConversationMessageContent, error) {
	return s.result, s.err
}

type conversationMessageChunkSinkStub struct {
	persistFn func(ctx context.Context, message domain.ConversationMessage, chunks []ProcessedConversationMessageChunk) error
}

func (s conversationMessageChunkSinkStub) PersistMessageChunks(ctx context.Context, message domain.ConversationMessage, chunks []ProcessedConversationMessageChunk) error {
	if s.persistFn == nil {
		return nil
	}
	return s.persistFn(ctx, message, chunks)
}

type conversationMessageCreateTransactionStub struct {
	runFn func(
		ctx context.Context,
		fn func(ctx context.Context, messageRepo port.ConversationMessageRepository, chunkSink ConversationMessageChunkSink) error,
	) error
}

func (s conversationMessageCreateTransactionStub) asFunc() ConversationMessageCreateTransaction {
	return func(
		ctx context.Context,
		fn func(ctx context.Context, messageRepo port.ConversationMessageRepository, chunkSink ConversationMessageChunkSink) error,
	) error {
		if s.runFn == nil {
			return fn(ctx, nil, nil)
		}
		return s.runFn(ctx, fn)
	}
}

func TestConversationMessageServiceAddMessageUsesContentProcessor(t *testing.T) {
	var created domain.ConversationMessage
	service := NewConversationMessageService(
		conversationMessageConversationRepoStub{getByConversationIDAndUserFn: func(context.Context, string, string) (domain.Conversation, error) {
			return domain.Conversation{}, nil
		}},
		conversationMessageRepoServiceStub{
			createFn: func(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
				created = message
				return message, nil
			},
			listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
				return nil, nil
			},
		},
		conversationSummaryRepoServiceStub{createFn: func(context.Context, domain.ConversationSummary) (domain.ConversationSummary, error) {
			return domain.ConversationSummary{}, nil
		}},
		nil,
	)
	service.SetContentProcessor(summaryProcessorStub{
		result: ProcessedConversationMessageContent{
			Content:        "压缩后的摘要",
			RawContent:     "原始超长文本",
			ContentSummary: "压缩后的摘要",
			IsSummarized:   true,
		},
	})

	_, err := service.AddMessage(context.Background(), AddConversationMessageInput{
		ConversationID: "c1",
		UserID:         "u1",
		Role:           convention.UserRole,
		Content:        "原始超长文本",
	})
	if err != nil {
		t.Fatalf("AddMessage returned error: %v", err)
	}
	if created.Content != "压缩后的摘要" || created.RawContent != "原始超长文本" || !created.IsSummarized {
		t.Fatalf("unexpected processed message: %#v", created)
	}
}

func TestConversationMessageServiceAddMessagePersistsSessionChunks(t *testing.T) {
	var (
		createdMessage domain.ConversationMessage
		persistedMsg   domain.ConversationMessage
		persisted      []ProcessedConversationMessageChunk
	)
	service := NewConversationMessageService(
		conversationMessageConversationRepoStub{getByConversationIDAndUserFn: func(context.Context, string, string) (domain.Conversation, error) {
			return domain.Conversation{}, nil
		}},
		conversationMessageRepoServiceStub{
			createFn: func(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
				createdMessage = message
				return message, nil
			},
			listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
				return nil, nil
			},
		},
		conversationSummaryRepoServiceStub{createFn: func(context.Context, domain.ConversationSummary) (domain.ConversationSummary, error) {
			return domain.ConversationSummary{}, nil
		}},
		nil,
	)
	service.SetContentProcessor(summaryProcessorStub{
		result: ProcessedConversationMessageContent{
			Content:        "summary",
			RawContent:     "original long message",
			ContentSummary: "summary",
			IsSummarized:   true,
			SessionChunks: []ProcessedConversationMessageChunk{
				{ChunkIndex: 1, Content: "chunk-1", ContentSummary: "summary-1", TokenEstimate: 10},
				{ChunkIndex: 2, Content: "chunk-2", ContentSummary: "summary-2", TokenEstimate: 12},
			},
		},
	})
	service.SetChunkSink(conversationMessageChunkSinkStub{
		persistFn: func(_ context.Context, message domain.ConversationMessage, chunks []ProcessedConversationMessageChunk) error {
			persistedMsg = message
			persisted = append([]ProcessedConversationMessageChunk(nil), chunks...)
			return nil
		},
	})

	_, err := service.AddMessage(context.Background(), AddConversationMessageInput{
		ConversationID: "c1",
		UserID:         "u1",
		Role:           convention.UserRole,
		Content:        "original long message",
	})
	if err != nil {
		t.Fatalf("AddMessage returned error: %v", err)
	}
	if persistedMsg.ID != createdMessage.ID {
		t.Fatalf("expected sink to receive created message id, got message=%#v created=%#v", persistedMsg, createdMessage)
	}
	if len(persisted) != 2 {
		t.Fatalf("expected 2 persisted chunks, got %#v", persisted)
	}
	if persisted[0].Content != "chunk-1" || persisted[1].ChunkIndex != 2 {
		t.Fatalf("unexpected persisted chunks: %#v", persisted)
	}
}

func TestConversationMessageServiceAddMessageUsesCreateTransaction(t *testing.T) {
	service := NewConversationMessageService(
		conversationMessageConversationRepoStub{getByConversationIDAndUserFn: func(context.Context, string, string) (domain.Conversation, error) {
			return domain.Conversation{}, nil
		}},
		conversationMessageRepoServiceStub{
			createFn: func(_ context.Context, _ domain.ConversationMessage) (domain.ConversationMessage, error) {
				t.Fatal("expected outer message repo to be bypassed when transaction is configured")
				return domain.ConversationMessage{}, nil
			},
			listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
				return nil, nil
			},
		},
		conversationSummaryRepoServiceStub{createFn: func(context.Context, domain.ConversationSummary) (domain.ConversationSummary, error) {
			return domain.ConversationSummary{}, nil
		}},
		nil,
	)
	service.SetContentProcessor(summaryProcessorStub{
		result: ProcessedConversationMessageContent{
			Content:        "summary",
			RawContent:     "original long message",
			ContentSummary: "summary",
			IsSummarized:   true,
			SessionChunks: []ProcessedConversationMessageChunk{
				{ChunkIndex: 1, Content: "chunk-1", ContentSummary: "summary-1", TokenEstimate: 10},
			},
		},
	})

	var (
		usedTxRepo  bool
		usedTxSink  bool
		persistedID string
	)
	txRepo := conversationMessageRepoServiceStub{
		createFn: func(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
			usedTxRepo = true
			message.ID = "tx-message-1"
			return message, nil
		},
		listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
			return nil, nil
		},
	}
	txSink := conversationMessageChunkSinkStub{
		persistFn: func(_ context.Context, message domain.ConversationMessage, chunks []ProcessedConversationMessageChunk) error {
			usedTxSink = true
			persistedID = message.ID
			if len(chunks) != 1 || chunks[0].Content != "chunk-1" {
				t.Fatalf("unexpected tx chunks: %#v", chunks)
			}
			return nil
		},
	}
	service.SetCreateTransaction(conversationMessageCreateTransactionStub{
		runFn: func(ctx context.Context, fn func(context.Context, port.ConversationMessageRepository, ConversationMessageChunkSink) error) error {
			return fn(ctx, txRepo, txSink)
		},
	}.asFunc())

	created, err := service.AddMessage(context.Background(), AddConversationMessageInput{
		ConversationID: "c1",
		UserID:         "u1",
		Role:           convention.UserRole,
		Content:        "original long message",
	})
	if err != nil {
		t.Fatalf("AddMessage returned error: %v", err)
	}
	if !usedTxRepo || !usedTxSink {
		t.Fatalf("expected transaction-scoped repo and sink to be used, got usedTxRepo=%v usedTxSink=%v", usedTxRepo, usedTxSink)
	}
	if created.ID != "tx-message-1" || persistedID != "tx-message-1" {
		t.Fatalf("expected created message id to flow through transaction, got created=%#v persistedID=%s", created, persistedID)
	}
}

func TestConversationMessageServiceListMessagesReturnsChronologicalResults(t *testing.T) {
	service := NewConversationMessageService(
		conversationMessageConversationRepoStub{getByConversationIDAndUserFn: func(context.Context, string, string) (domain.Conversation, error) {
			return domain.Conversation{ID: "1", ConversationID: "c1", UserID: "u1"}, nil
		}},
		conversationMessageRepoServiceStub{
			createFn: func(context.Context, domain.ConversationMessage) (domain.ConversationMessage, error) {
				return domain.ConversationMessage{}, nil
			},
			listFn: func(_ context.Context, filter port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
				if filter.Order != port.ConversationMessageOrderDesc {
					t.Fatalf("expected desc query order, got %s", filter.Order)
				}
				return []domain.ConversationMessage{
					{ID: "2", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "第二条"},
					{ID: "1", ConversationID: "c1", UserID: "u1", Role: "user", Content: "第一条"},
				}, nil
			},
		},
		conversationSummaryRepoServiceStub{createFn: func(context.Context, domain.ConversationSummary) (domain.ConversationSummary, error) {
			return domain.ConversationSummary{}, nil
		}},
		messageFeedbackRepoServiceStub{
			listFn: func(_ context.Context, userID string, messageIDs []string) ([]domain.MessageFeedback, error) {
				if userID != "u1" || len(messageIDs) != 1 || messageIDs[0] != "2" {
					t.Fatalf("unexpected feedback lookup: userID=%s messageIDs=%v", userID, messageIDs)
				}
				return []domain.MessageFeedback{{MessageID: "2", Vote: 1}}, nil
			},
		},
	)

	items, err := service.ListMessages(context.Background(), ListConversationMessagesInput{
		ConversationID: "c1",
		UserID:         "u1",
		Limit:          2,
		Order:          port.ConversationMessageOrderDesc,
	})
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != "1" || items[1].ID != "2" {
		t.Fatalf("expected chronological order, got %#v", items)
	}
	if items[1].Vote == nil || *items[1].Vote != 1 {
		t.Fatalf("expected assistant vote, got %#v", items[1].Vote)
	}
}

func TestConversationMessageServiceListMessagesReturnsEmptyWhenConversationMissing(t *testing.T) {
	service := NewConversationMessageService(
		conversationMessageConversationRepoStub{getByConversationIDAndUserFn: func(context.Context, string, string) (domain.Conversation, error) {
			return domain.Conversation{}, nil
		}},
		conversationMessageRepoServiceStub{
			createFn: func(context.Context, domain.ConversationMessage) (domain.ConversationMessage, error) {
				return domain.ConversationMessage{}, nil
			},
			listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
				return nil, errors.New("should not be called")
			},
		},
		conversationSummaryRepoServiceStub{createFn: func(context.Context, domain.ConversationSummary) (domain.ConversationSummary, error) {
			return domain.ConversationSummary{}, nil
		}},
		nil,
	)

	items, err := service.ListMessages(context.Background(), ListConversationMessagesInput{
		ConversationID: "c1",
		UserID:         "u1",
	})
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty result, got %#v", items)
	}
}

func TestConversationMessageServiceAddMessageSummaryCreatesRecord(t *testing.T) {
	var created domain.ConversationSummary
	service := NewConversationMessageService(
		conversationMessageConversationRepoStub{getByConversationIDAndUserFn: func(context.Context, string, string) (domain.Conversation, error) {
			return domain.Conversation{}, nil
		}},
		conversationMessageRepoServiceStub{
			createFn: func(context.Context, domain.ConversationMessage) (domain.ConversationMessage, error) {
				return domain.ConversationMessage{}, nil
			},
			listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
				return nil, nil
			},
		},
		conversationSummaryRepoServiceStub{
			createFn: func(_ context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error) {
				created = summary
				return summary, nil
			},
		},
		nil,
	)

	result, err := service.AddMessageSummary(context.Background(), AddConversationSummaryInput{
		ConversationID: "c1",
		UserID:         "u1",
		Content:        "摘要内容",
		LastMessageID:  "m2",
	})
	if err != nil {
		t.Fatalf("AddMessageSummary returned error: %v", err)
	}
	if result.ID == "" || created.ID == "" {
		t.Fatal("expected generated summary id")
	}
	if created.Content != "摘要内容" || created.LastMessageID != "m2" {
		t.Fatalf("unexpected summary: %#v", created)
	}
}

func TestConversationMessageServiceGetLatestSummary(t *testing.T) {
	service := NewConversationMessageService(
		conversationMessageConversationRepoStub{getByConversationIDAndUserFn: func(context.Context, string, string) (domain.Conversation, error) {
			return domain.Conversation{}, nil
		}},
		conversationMessageRepoServiceStub{
			createFn: func(context.Context, domain.ConversationMessage) (domain.ConversationMessage, error) {
				return domain.ConversationMessage{}, nil
			},
			listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
				return nil, nil
			},
		},
		conversationSummaryRepoServiceStub{
			createFn: func(context.Context, domain.ConversationSummary) (domain.ConversationSummary, error) {
				return domain.ConversationSummary{}, nil
			},
			latestFn: func(_ context.Context, _ string, _ string) (domain.ConversationSummary, error) {
				return domain.ConversationSummary{ID: "s1", Content: "摘要"}, nil
			},
		},
		nil,
	)

	summary, err := service.GetLatestSummary(context.Background(), GetLatestConversationSummaryInput{
		ConversationID: "c1",
		UserID:         "u1",
	})
	if err != nil {
		t.Fatalf("GetLatestSummary returned error: %v", err)
	}
	if summary.ID != "s1" || summary.Content != "摘要" {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}
