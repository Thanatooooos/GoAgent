package history

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
	infraai "local/rag-project/internal/infra-ai"
)

// TestCompressSummaryIntegration 使用真实 LLM 验证对话摘要压缩效果。
// 设置 RAG_INTEGRATION_API=1 启用。
func TestCompressSummaryIntegration(t *testing.T) {
	if os.Getenv("RAG_INTEGRATION_API") != "1" {
		t.Skip("set RAG_INTEGRATION_API=1 to run real API integration test")
	}

	aiRuntime := infraai.NewRuntime()

	// 先探测 chat 服务是否可用，不可用时跳过而非失败。
	if _, err := aiRuntime.Chat.Chat("ping"); err != nil && strings.Contains(err.Error(), "no") {
		t.Skipf("chat service not available, skipping compression integration test: %v", err)
	}

	t.Run("CompressGeneratesSummary", func(t *testing.T) {
		// 构造一段多轮对话，验证 LLM 能生成合理摘要。
		summaryRepo := &integrationSummaryRepo{}
		messageRepo := &integrationMessageRepo{
			messages: []domain.ConversationMessage{
				{ID: "10", ConversationID: "ci1", UserID: "u1", Role: "assistant", Content: "Go 的并发模型基于 goroutine 和 channel，goroutine 是一种轻量级线程，channel 用于 goroutine 之间的通信。"},
				{ID: "9", ConversationID: "ci1", UserID: "u1", Role: "user", Content: "Go 语言的并发模型是怎样的"},
				{ID: "8", ConversationID: "ci1", UserID: "u1", Role: "assistant", Content: "Go 语言的主要特点包括：静态类型、垃圾回收、内置并发支持、简洁的语法和快速的编译速度。"},
				{ID: "7", ConversationID: "ci1", UserID: "u1", Role: "user", Content: "Go 语言有哪些主要特点"},
			},
		}

		svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
			MessageRepo: messageRepo,
			ChatService: aiRuntime.Chat,
			StartTurns:  2, // 阈值为 4，现有 4 条正好触发。
			MaxChars:    200,
		})

		err := svc.CompressIfNeeded(context.Background(), "ci1", "u1", convention.UserMessage("谢谢"))
		if err != nil {
			t.Fatalf("CompressIfNeeded failed: %v", err)
		}
		if !summaryRepo.created {
			t.Fatal("expected summary to be created")
		}

		t.Logf("generated summary: %q", summaryRepo.lastContent)

		summary := summaryRepo.lastContent
		if len(summary) == 0 {
			t.Fatal("expected non-empty summary")
		}
		// 摘要应合理简短（不超过 maxChars 的 1.5 倍，给 LLM 一些容差）。
		if len([]rune(summary)) > 300 {
			t.Logf("warning: summary may be too long (%d runes): %s", len([]rune(summary)), summary)
		}
		// 摘要应包含对话的关键信息。
		hasGo := strings.Contains(strings.ToLower(summary), "go") || strings.Contains(summary, "并发") || strings.Contains(summary, "特点")
		if !hasGo {
			t.Logf("warning: summary may not capture key content: %s", summary)
		}
	})

	t.Run("CompressWithExistingSummary", func(t *testing.T) {
		// 已有摘要且最后一条消息已被覆盖 → 不重复压缩。
		summaryRepo := &integrationSummaryRepo{
			latestSummary: domain.ConversationSummary{
				ID:            "existing",
				LastMessageID: "10",
				Content:       "之前的摘要",
			},
		}
		messageRepo := &integrationMessageRepo{
			messages: []domain.ConversationMessage{
				{ID: "10", ConversationID: "ci2", UserID: "u1", Role: "assistant", Content: "最后一条"},
				{ID: "9", ConversationID: "ci2", UserID: "u1", Role: "user", Content: "倒数第二条"},
				{ID: "8", ConversationID: "ci2", UserID: "u1", Role: "assistant", Content: "倒数第三条"},
				{ID: "7", ConversationID: "ci2", UserID: "u1", Role: "user", Content: "倒数第四条"},
			},
		}

		svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
			MessageRepo: messageRepo,
			ChatService: aiRuntime.Chat,
			StartTurns:  2,
			MaxChars:    200,
		})

		err := svc.CompressIfNeeded(context.Background(), "ci2", "u1", convention.UserMessage("再来一条"))
		if err != nil {
			t.Fatalf("CompressIfNeeded failed: %v", err)
		}
		if summaryRepo.created {
			t.Fatal("expected no duplicate compression when latest message already covered")
		}
		t.Log("correctly skipped duplicate compression")
	})

	t.Run("CompressNoChatService", func(t *testing.T) {
		// 无 LLM 服务时静默跳过。
		adapter := NewSummaryServiceAdapter(nil)
		err := adapter.CompressIfNeeded(context.Background(), "c", "u", convention.UserMessage("msg"))
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}

// integrationSummaryRepo 真实 API 集成测试用摘要仓储桩。
type integrationSummaryRepo struct {
	created       bool
	lastContent   string
	latestSummary domain.ConversationSummary
}

func (r *integrationSummaryRepo) Create(_ context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error) {
	r.created = true
	r.lastContent = summary.Content
	summary.ID = "summary-integration"
	return summary, nil
}

func (r *integrationSummaryRepo) FindLatestByConversationIDAndUserID(_ context.Context, _ string, _ string) (domain.ConversationSummary, error) {
	return r.latestSummary, nil
}

func (r *integrationSummaryRepo) DeleteByConversationIDAndUserID(_ context.Context, _ string, _ string) error {
	return nil
}

var _ port.ConversationSummaryRepository = (*integrationSummaryRepo)(nil)

// integrationMessageRepo 真实 API 集成测试用消息仓储桩。
type integrationMessageRepo struct {
	messages []domain.ConversationMessage
}

func (r *integrationMessageRepo) Create(_ context.Context, msg domain.ConversationMessage) (domain.ConversationMessage, error) {
	return msg, nil
}

func (r *integrationMessageRepo) GetByID(_ context.Context, _ string) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, nil
}

func (r *integrationMessageRepo) List(_ context.Context, filter port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
	if filter.Limit > 0 && filter.Limit < len(r.messages) {
		return r.messages[:filter.Limit], nil
	}
	return r.messages, nil
}

func (r *integrationMessageRepo) CountByConversationIDAndUserIDAndRole(_ context.Context, _ string, _ string, role string) (int64, error) {
	var count int64
	for _, msg := range r.messages {
		if msg.Role == role {
			count++
		}
	}
	return count, nil
}

func (r *integrationMessageRepo) FindMaxIDAtOrBefore(_ context.Context, _ string, _ string, _ time.Time) (string, error) {
	return "", nil
}

func (r *integrationMessageRepo) DeleteByConversationIDAndUserID(_ context.Context, _ string, _ string) error {
	return nil
}

var _ port.ConversationMessageRepository = (*integrationMessageRepo)(nil)
