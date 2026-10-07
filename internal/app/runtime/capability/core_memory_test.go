package capability

import (
	"context"
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

type coreMemoryStub struct {
	userID, messageID, memoryID, content string
}

func (s *coreMemoryStub) AddCoreMemory(_ context.Context, userID, messageID, content string) (domain.MemoryItem, error) {
	s.userID, s.messageID, s.content = userID, messageID, content
	return domain.MemoryItem{ID: "new", Content: content, Status: domain.MemoryStatusActive}, nil
}
func (s *coreMemoryStub) UpdateCoreMemory(_ context.Context, userID, memoryID, messageID, content string) (domain.MemoryItem, error) {
	s.userID, s.memoryID, s.messageID, s.content = userID, memoryID, messageID, content
	return domain.MemoryItem{ID: "new", Content: content, Status: domain.MemoryStatusActive}, nil
}
func (s *coreMemoryStub) DeleteCoreMemory(_ context.Context, userID, memoryID string) (domain.MemoryItem, error) {
	s.userID, s.memoryID = userID, memoryID
	return domain.MemoryItem{ID: memoryID, Status: domain.MemoryStatusExpired}, nil
}

func TestCoreMemoryToolsBindRuntimeIdentity(t *testing.T) {
	t.Parallel()
	service := &coreMemoryStub{}
	ctx := Context{Context: context.Background(), UserID: "user-1", UserMessageID: "message-1", AllowMemoryMutation: true}
	result, err := CoreMemoryAdd(service).Execute(Value(`{"content":"reply in Chinese"}`), ctx)
	if err != nil || result.Content == "" {
		t.Fatalf("add = (%+v, %v)", result, err)
	}
	if service.userID != "user-1" || service.messageID != "message-1" || service.content != "reply in Chinese" {
		t.Fatalf("bound input = %#v", service)
	}
	if _, err := CoreMemoryDelete(service).Describe(Value(`{"memory_id":"m-1"}`), Context{}); err == nil {
		t.Fatal("mutation without explicit permission must be denied")
	}
}

func TestCoreMemoryToolsRejectUnknownFields(t *testing.T) {
	t.Parallel()
	if err := CoreMemoryAdd(&coreMemoryStub{}).Validate(Value(`{"content":"x","user_id":"other"}`)); err == nil {
		t.Fatal("unexpected field must be rejected")
	}
}
