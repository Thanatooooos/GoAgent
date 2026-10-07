package pgvector

import "context"

// Unspecified scope is a public-source query, never a query of private Work
// containers. Runtime separately authorizes every explicit set of KB IDs.
func (s *VectorStore) defaultPublicScope(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) > 0 {
		return ids, nil
	}
	public := []string{}
	err := s.db.WithContext(ctx).Raw(`SELECT id FROM t_knowledge_base WHERE deleted=0 AND work_private=false`).Scan(&public).Error
	return public, err
}
