package service

import "local/rag-project/internal/app/work/port"

// Service exposes the owner-scoped manual operations. AI collaboration extends
// this boundary with accepted turns rather than adding another document store.
type Service struct{ port.Repository }

func New(repository port.Repository) *Service { return &Service{Repository: repository} }
