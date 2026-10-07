package longtermmemory

import (
	"context"
	"strings"
	"unicode/utf8"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/app/rag/service/longtermmemory/governance"
	"local/rag-project/internal/framework/exception"
)

const maxCoreMemoryContentRunes = 400

// LoadCoreMemoryContext returns the small, stable preference set injected into
// a runtime session. It deliberately excludes facts, pending candidates, and
// KB-scoped memories.
func (s *MemoryService) LoadCoreMemoryContext(ctx context.Context, userID string) (string, error) {
	result, err := s.RecallMemories(ctx, RecallMemoriesInput{
		UserID: userID, ScopeTypes: []string{domain.MemoryScopeGlobal},
		MemoryTypes: []string{domain.MemoryTypePreference}, Statuses: []string{domain.MemoryStatusActive},
	})
	if err != nil {
		return "", err
	}
	return strings.Replace(result.Context, "Rule Memories:", "User core memories:", 1), nil
}

func (s *MemoryService) AddCoreMemory(ctx context.Context, userID, sourceMessageID, content string) (domain.MemoryItem, error) {
	input, err := normalizeCoreMemoryInput(userID, sourceMessageID, content)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	if err := s.rejectDuplicateCoreMemory(ctx, input.UserID, input.Content, ""); err != nil {
		return domain.MemoryItem{}, err
	}
	return s.SaveExplicitMemory(ctx, input)
}

func (s *MemoryService) UpdateCoreMemory(ctx context.Context, userID, memoryID, sourceMessageID, content string) (domain.MemoryItem, error) {
	input, err := normalizeCoreMemoryInput(userID, sourceMessageID, content)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return domain.MemoryItem{}, exception.NewClientException("memory id is required", nil)
	}
	if err := s.rejectDuplicateCoreMemory(ctx, input.UserID, input.Content, memoryID); err != nil {
		return domain.MemoryItem{}, err
	}
	var created domain.MemoryItem
	err = s.runMemoryMutation(ctx, func(ctx context.Context, repo port.MemoryItemRepository) error {
		previous, err := repo.GetByID(ctx, memoryID)
		if err != nil {
			return exception.NewServiceException("failed to load memory item", err)
		}
		if !isOwnedActiveCoreMemory(previous, input.UserID) {
			return exception.NewClientException("core memory not found", nil)
		}
		now := s.now()
		previous.Status = domain.MemoryStatusSuperseded
		previous.UpdatedBy = input.UserID
		previous.UpdateTime = now
		if _, err := repo.Update(ctx, previous); err != nil {
			return exception.NewServiceException("failed to supersede core memory", err)
		}
		created, err = governance.SaveExplicitMemoryWithRepo(ctx, repo, input, s.now)
		if err != nil {
			return err
		}
		created.SupersedesID = previous.ID
		created, err = repo.Update(ctx, created)
		if err != nil {
			return exception.NewServiceException("failed to link superseded core memory", err)
		}
		return nil
	})
	if err != nil {
		return domain.MemoryItem{}, err
	}
	s.persistMemoryEmbedding(ctx, created)
	s.bumpRecallCacheVersion(ctx, created)
	return created, nil
}

func (s *MemoryService) DeleteCoreMemory(ctx context.Context, userID, memoryID string) (domain.MemoryItem, error) {
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return domain.MemoryItem{}, exception.NewClientException("memory id is required", nil)
	}
	item, err := s.repo.GetByID(ctx, memoryID)
	if err != nil {
		return domain.MemoryItem{}, exception.NewServiceException("failed to load memory item", err)
	}
	if !isOwnedActiveCoreMemory(item, userID) {
		return domain.MemoryItem{}, exception.NewClientException("core memory not found", nil)
	}
	return s.ExpireMemory(ctx, userID, memoryID)
}

func normalizeCoreMemoryInput(userID, sourceMessageID, content string) (SaveExplicitMemoryInput, error) {
	content = strings.TrimSpace(strings.Join(strings.Fields(content), " "))
	if content == "" {
		return SaveExplicitMemoryInput{}, exception.NewClientException("core memory content is required", nil)
	}
	if utf8.RuneCountInString(content) > maxCoreMemoryContentRunes {
		return SaveExplicitMemoryInput{}, exception.NewClientException("core memory content is too long", nil)
	}
	return SaveExplicitMemoryInput{
		UserID: strings.TrimSpace(userID), ScopeType: domain.MemoryScopeGlobal,
		MemoryType: domain.MemoryTypePreference, Category: domain.MemoryCategoryBehavior,
		ValueType: domain.MemoryValueTypeText, Content: content, Summary: content,
		SourceMessageID: strings.TrimSpace(sourceMessageID), ExtractionMethod: domain.MemoryExtractionMethodLLM,
	}, nil
}

func (s *MemoryService) rejectDuplicateCoreMemory(ctx context.Context, userID, content, excludedID string) error {
	items, err := s.repo.List(ctx, port.MemoryItemListFilter{UserID: userID, ScopeTypes: []string{domain.MemoryScopeGlobal}, MemoryTypes: []string{domain.MemoryTypePreference}, Statuses: []string{domain.MemoryStatusActive}, ListOptions: port.ListOptions{Limit: 20}})
	if err != nil {
		return exception.NewServiceException("failed to list core memories", err)
	}
	for _, item := range items {
		if item.ID != excludedID && strings.EqualFold(strings.TrimSpace(item.Content), content) {
			return exception.NewClientException("an identical core memory already exists", nil)
		}
	}
	return nil
}

func isOwnedActiveCoreMemory(item domain.MemoryItem, userID string) bool {
	return item.ID != "" && strings.TrimSpace(item.UserID) == strings.TrimSpace(userID) && item.ScopeType == domain.MemoryScopeGlobal && item.MemoryType == domain.MemoryTypePreference && item.Status == domain.MemoryStatusActive
}
