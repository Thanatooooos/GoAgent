package dailybrief

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/dailybrief/models"
	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type IssueRepository struct {
	db *gorm.DB
}

func NewIssueRepository(db *gorm.DB) *IssueRepository {
	return &IssueRepository{db: db}
}

func (r *IssueRepository) Create(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	model := toIssueModel(issue)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.Issue{}, fmt.Errorf("create daily brief issue: %w", err)
	}
	return toIssueDomain(model), nil
}

func (r *IssueRepository) Update(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	model := toIssueModel(issue)
	result := r.db.WithContext(ctx).
		Model(&models.IssueModel{}).
		Where("id = ?", issue.ID).
		Updates(map[string]any{
			"status":           model.Status,
			"headline":         model.Headline,
			"top_summary":      model.TopSummary,
			"sections_json":    model.SectionsJSON,
			"item_count":       model.ItemCount,
			"published_run_id": model.PublishedRunID,
			"generated_at":     model.GeneratedAt,
			"published_at":     model.PublishedAt,
			"update_time":      model.UpdateTime,
		})
	if result.Error != nil {
		return domain.Issue{}, fmt.Errorf("update daily brief issue: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.Issue{}, fmt.Errorf("update daily brief issue: no rows affected")
	}
	return issue, nil
}

func (r *IssueRepository) GetByID(ctx context.Context, id string) (domain.Issue, error) {
	var model models.IssueModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Issue{}, nil
	}
	if err != nil {
		return domain.Issue{}, fmt.Errorf("get daily brief issue by id: %w", err)
	}
	return toIssueDomain(model), nil
}

func (r *IssueRepository) GetByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.Issue, error) {
	var model models.IssueModel
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("brief_date = ?", briefDate).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.scheduledIssueState(ctx, userID, briefDate, domain.Issue{})
	}
	if err != nil {
		return domain.Issue{}, fmt.Errorf("get daily brief issue by user and brief date: %w", err)
	}
	issue := toIssueDomain(model)
	if issue.Status == domain.IssueStatusReady {
		return issue, nil
	}
	return r.scheduledIssueState(ctx, userID, briefDate, issue)
}

// Unpublished runs have no issue artifact. Project their page state at read
// time, so failed/uncertain runs do not become an unexplained empty page.
func (r *IssueRepository) scheduledIssueState(ctx context.Context, userID, date string, fallback domain.Issue) (domain.Issue, error) {
	var run struct {
		ID, Status  string
		ScheduledAt time.Time
	}
	result := r.db.WithContext(ctx).Raw(`SELECT o.id,o.status,o.scheduled_at FROM t_daily_brief_task_binding b
		JOIN t_scheduled_task t ON t.id=b.task_id
		JOIN t_scheduled_task_occurrence o ON o.task_id=t.id AND o.version=t.current_version
		JOIN t_scheduled_task_version v ON v.task_id=o.task_id AND v.version=o.version
		WHERE b.user_id=? AND t.deleted_at IS NULL
		AND (o.scheduled_at AT TIME ZONE (v.schedule_json->>'timezone'))::date::text=?
		AND o.status IN ('running','retry','ready','failed','uncertain','missed')
		ORDER BY o.scheduled_at DESC LIMIT 1`, userID, date).Scan(&run)
	if result.Error != nil {
		return domain.Issue{}, result.Error
	}
	if result.RowsAffected == 0 {
		return fallback, nil
	}
	issue := domain.NewIssue(run.ID, userID, date)
	if run.Status == "failed" || run.Status == "uncertain" || run.Status == "missed" {
		issue.Status = domain.IssueStatusFailed
	}
	return issue, nil
}

func (r *IssueRepository) List(ctx context.Context, filter port.IssueListFilter) ([]domain.Issue, error) {
	query := r.db.WithContext(ctx).Model(&models.IssueModel{}).Order("brief_date desc")
	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.BriefDate != "" {
		query = query.Where("brief_date = ?", filter.BriefDate)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []models.IssueModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list daily brief issues: %w", err)
	}
	result := make([]domain.Issue, 0, len(items))
	for _, item := range items {
		result = append(result, toIssueDomain(item))
	}
	return result, nil
}

var _ port.IssueRepository = (*IssueRepository)(nil)
