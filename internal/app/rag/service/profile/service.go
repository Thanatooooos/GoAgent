package profile

import (
	"context"
	"strings"

	"local/rag-project/internal/app/rag/domain"
)

type Repository interface {
	Get(context.Context, string) (domain.UserMemoryProfile, error)
	Save(context.Context, domain.UserMemoryProfile) (domain.UserMemoryProfile, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Get returns the current derived profile. It is intentionally read-only: only
// the observation worker is allowed to call ApplyObservation.
func (s *Service) Get(ctx context.Context, userID string) (domain.UserMemoryProfile, error) {
	if s == nil || s.repo == nil {
		return domain.UserMemoryProfile{UserID: strings.TrimSpace(userID), Version: 1}, nil
	}
	return s.repo.Get(ctx, strings.TrimSpace(userID))
}

func (s *Service) LoadContext(ctx context.Context, userID string) (string, error) {
	if s == nil || s.repo == nil {
		return "", nil
	}
	profile, err := s.repo.Get(ctx, strings.TrimSpace(userID))
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(profile.ContentMarkdown)
	if content == "" {
		return "", nil
	}
	return profileContextInstruction + content, nil
}
