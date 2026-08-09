package domain

import "time"

const (
	WikiPageTypeSummary = "summary"
	WikiPageTypeEntity  = "entity"
	WikiPageTypeConcept = "concept"
	WikiPageTypeIndex   = "index"
)

const (
	WikiPageStatusDraft     = "draft"
	WikiPageStatusPublished = "published"
)

type WikiPage struct {
	ID                string
	KnowledgeBaseID   string
	Slug              string
	Title             string
	PageType          string
	Status            string
	Content           string
	Summary           string
	SourceDocumentIDs []string
	SourceChunkIDs    []string
	CreatedBy         string
	UpdatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// NewWikiPage 构造默认已发布状态的页面。
func NewWikiPage(id, kbID, slug, title, pageType, content, createdBy string) WikiPage {
	now := time.Now()
	return WikiPage{
		ID:              id,
		KnowledgeBaseID: kbID,
		Slug:            slug,
		Title:           title,
		PageType:        pageType,
		Status:          WikiPageStatusPublished,
		Content:         content,
		CreatedBy:       createdBy,
		UpdatedBy:       createdBy,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
