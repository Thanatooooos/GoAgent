package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type ChunkJobs struct{ db *gorm.DB }

var _ port.ChunkJobs = (*ChunkJobs)(nil)

func NewChunkJobs(db *gorm.DB) *ChunkJobs { return &ChunkJobs{db: db} }

type chunkJobRow struct {
	ID               string
	DocumentID       string
	Epoch            int64
	TriggeredBy      string
	DocumentSnapshot []byte
	SourceFileURL    string
	Refreshed        bool
	State            string
	Owner            string
}

func (r chunkJobRow) job() (port.ChunkJob, error) {
	j := port.ChunkJob{ChunkDocumentTask: port.ChunkDocumentTask{TaskID: r.ID, DocumentID: r.DocumentID, TriggeredBy: r.TriggeredBy}, Epoch: r.Epoch, Owner: r.Owner, Refreshed: r.Refreshed}
	j.SourceFileURL = r.SourceFileURL
	err := json.Unmarshal(r.DocumentSnapshot, &j.Document)
	return j, err
}
func lockChunkDocument(tx *gorm.DB, id string) error {
	var found string
	if err := tx.Raw(`SELECT id FROM t_knowledge_document WHERE id=? AND deleted=0 FOR UPDATE`, id).Scan(&found).Error; err != nil {
		return err
	}
	if found == "" {
		return port.ErrChunkLeaseLost
	}
	return nil
}
func (s *ChunkJobs) Admit(ctx context.Context, task port.ChunkDocumentTask, refreshed *domain.KnowledgeDocument) (job port.ChunkJob, err error) {
	if task.TaskID == "" || task.DocumentID == "" || task.TriggeredBy == "" {
		return job, fmt.Errorf("chunk job identity is required")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockChunkDocument(tx, task.DocumentID); err != nil {
			return err
		}
		doc, err := NewKnowledgeDocumentRepository(tx, nil).GetByID(ctx, task.DocumentID)
		if err != nil {
			return err
		}
		if !doc.Enabled {
			return fmt.Errorf("knowledge document is disabled")
		}
		sourceFileURL := doc.FileURL
		var active int64
		if err := tx.Table("t_document_chunk_job").Where("document_id=? AND state IN ('pending','running')", doc.ID).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return fmt.Errorf("knowledge document processing is already running")
		}
		if refreshed == nil {
			if !doc.CanStartProcessing() {
				return fmt.Errorf("knowledge document cannot start processing")
			}
		} else {
			if doc.Status != "running" || !doc.UpdatedAt.Equal(refreshed.UpdatedAt) {
				return port.ErrChunkLeaseLost
			}
			doc = *refreshed
		}
		var epoch int64
		if err := tx.Raw(`UPDATE t_knowledge_document SET chunk_epoch=chunk_epoch+1,current_chunk_job_id=?,status='running',updated_by=?,update_time=CURRENT_TIMESTAMP WHERE id=? RETURNING chunk_epoch`, task.TaskID, task.TriggeredBy, task.DocumentID).Scan(&epoch).Error; err != nil {
			return err
		}
		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO t_document_chunk_job(id,document_id,epoch,triggered_by,document_snapshot,source_file_url,refreshed,state) VALUES(?,?,?,?,CAST(? AS jsonb),?,?,'pending')`, task.TaskID, task.DocumentID, epoch, task.TriggeredBy, string(data), sourceFileURL, refreshed != nil).Error; err != nil {
			return err
		}
		job = port.ChunkJob{ChunkDocumentTask: task, Epoch: epoch, Document: doc, SourceFileURL: sourceFileURL, Refreshed: refreshed != nil}
		return nil
	})
	return
}
func (s *ChunkJobs) Claim(ctx context.Context, id, owner string, ttl time.Duration) (job *port.ChunkJob, err error) {
	if owner == "" || ttl <= 0 {
		return nil, fmt.Errorf("chunk owner and lease duration are required")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row chunkJobRow
		if err := tx.Raw(`SELECT * FROM t_document_chunk_job WHERE id=?`, id).Scan(&row).Error; err != nil {
			return err
		}
		if row.ID == "" {
			return nil
		}
		if err := lockChunkDocument(tx, row.DocumentID); err != nil {
			return err
		}
		result := tx.Exec(`UPDATE t_document_chunk_job j SET state='running',owner=?,lease_until=clock_timestamp()+?*INTERVAL '1 second',heartbeat_at=clock_timestamp(),updated_at=clock_timestamp() FROM t_knowledge_document d WHERE j.id=? AND j.state='pending' AND d.id=j.document_id AND d.current_chunk_job_id=j.id AND d.chunk_epoch=j.epoch AND d.enabled=1 AND d.deleted=0 AND d.status='running'`, owner, ttl.Seconds(), id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		row.Owner = owner
		j, err := row.job()
		if err != nil {
			return err
		}
		job = &j
		return nil
	})
	return
}
func (s *ChunkJobs) Renew(ctx context.Context, job port.ChunkJob, ttl time.Duration) error {
	result := s.db.WithContext(ctx).Exec(`UPDATE t_document_chunk_job j SET lease_until=clock_timestamp()+?*INTERVAL '1 second',heartbeat_at=clock_timestamp(),updated_at=clock_timestamp() FROM t_knowledge_document d WHERE j.id=? AND j.owner=? AND j.epoch=? AND j.state='running' AND j.lease_until>clock_timestamp() AND d.id=j.document_id AND d.current_chunk_job_id=j.id AND d.chunk_epoch=j.epoch AND d.deleted=0 AND d.enabled=1 AND d.status='running'`, ttl.Seconds(), job.TaskID, job.Owner, job.Epoch)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return port.ErrChunkLeaseLost
	}
	return nil
}

// checkChunkJob is called with the document lock held, in the publication transaction.
func checkChunkJob(ctx context.Context, tx *gorm.DB, job port.ChunkJob) error {
	var found string
	err := tx.Raw(`SELECT j.id FROM t_document_chunk_job j JOIN t_knowledge_document d ON d.id=j.document_id WHERE j.id=? AND j.owner=? AND j.epoch=? AND j.state='running' AND j.lease_until>clock_timestamp() AND d.id=? AND d.current_chunk_job_id=j.id AND d.chunk_epoch=j.epoch AND d.deleted=0 AND d.enabled=1 AND d.status='running' FOR UPDATE OF j`, job.TaskID, job.Owner, job.Epoch, job.DocumentID).Scan(&found).Error
	if err != nil {
		return err
	}
	if found == "" {
		return port.ErrChunkLeaseLost
	}
	return nil
}

func validateChunkSnapshot(ctx context.Context, tx *gorm.DB, job port.ChunkJob) error {
	actual, err := NewKnowledgeDocumentRepository(tx, nil).GetByID(ctx, job.DocumentID)
	if err != nil {
		return err
	}
	if actual.FileURL != job.SourceFileURL || actual.ChunkStrategy != job.Document.ChunkStrategy || string(actual.ChunkConfig) != string(job.Document.ChunkConfig) || actual.ProcessMode != job.Document.ProcessMode {
		return port.ErrChunkLeaseLost
	}
	return nil
}
func (r *KnowledgeDocumentRepository) FenceChunkJob(ctx context.Context) error {
	job, ok := port.CurrentChunkJob(ctx)
	if !ok {
		return nil
	}
	if err := checkChunkJob(ctx, r.db.WithContext(ctx), job); err != nil {
		return err
	}
	return validateChunkSnapshot(ctx, r.db.WithContext(ctx), job)
}
func (r *KnowledgeDocumentRepository) CompleteChunkJob(ctx context.Context, log domain.KnowledgeDocumentChunkLog, status string) error {
	job, ok := port.CurrentChunkJob(ctx)
	if !ok {
		return fmt.Errorf("chunk execution is required")
	}
	if err := checkChunkJob(ctx, r.db.WithContext(ctx), job); err != nil {
		return err
	}
	if err := validateChunkSnapshot(ctx, r.db.WithContext(ctx), job); err != nil {
		return err
	}
	persisted, err := NewKnowledgeDocumentChunkLogRepository(r.db).GetByID(ctx, log.ID)
	if err != nil {
		return err
	}
	log.StartTime = persisted.StartTime
	if log.StartTime != nil && log.EndTime != nil {
		log.TotalDuration = log.EndTime.Sub(*log.StartTime).Milliseconds()
	}
	if _, err := NewKnowledgeDocumentChunkLogRepository(r.db).Update(ctx, log); err != nil {
		return err
	}
	if job.Refreshed {
		if err := r.db.WithContext(ctx).Exec(`UPDATE t_knowledge_document SET doc_name=?,file_url=?,file_type=?,file_size=? WHERE id=?`, job.Document.Name, job.Document.FileURL, job.Document.FileType, job.Document.FileSize, job.DocumentID).Error; err != nil {
			return err
		}
	}
	if err := r.db.WithContext(ctx).Exec(`UPDATE t_knowledge_document SET status=?,updated_by=?,update_time=CURRENT_TIMESTAMP WHERE id=?`, status, job.TriggeredBy, job.DocumentID).Error; err != nil {
		return err
	}
	// The lease may expire while writing the projection. Check wall-clock time
	// again at the final write so the whole transaction rolls back in that case.
	completed := r.db.WithContext(ctx).Exec(`UPDATE t_document_chunk_job SET state='completed',lease_until=NULL,updated_at=clock_timestamp() WHERE id=? AND owner=? AND epoch=? AND state='running' AND lease_until>clock_timestamp()`, job.TaskID, job.Owner, job.Epoch)
	if completed.Error != nil {
		return completed.Error
	}
	if completed.RowsAffected != 1 {
		return port.ErrChunkLeaseLost
	}
	return nil
}
func (s *ChunkJobs) Fail(ctx context.Context, job port.ChunkJob, cause string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockChunkDocument(tx, job.DocumentID); err != nil {
			return err
		}
		if err := checkChunkJob(ctx, tx, job); err != nil {
			return err
		}
		return interruptChunkJob(tx, job.TaskID, job.DocumentID, "failed", cause)
	})
}
func interruptChunkJob(tx *gorm.DB, id, document, state, cause string) error {
	if err := tx.Exec(`UPDATE t_document_chunk_job SET state=?,error=?,lease_until=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=?`, state, cause, id).Error; err != nil {
		return err
	}
	if err := tx.Exec(`UPDATE t_knowledge_document SET status='failed',update_time=CURRENT_TIMESTAMP WHERE id=? AND current_chunk_job_id=? AND status='running'`, document, id).Error; err != nil {
		return err
	}
	return tx.Exec(`UPDATE t_knowledge_document_chunk_log SET status='failed',error_message=?,end_time=CURRENT_TIMESTAMP,update_time=CURRENT_TIMESTAMP WHERE id=? AND status='running'`, cause, id).Error
}
func (s *ChunkJobs) Pending(ctx context.Context, limit int) ([]port.ChunkDocumentTask, error) {
	var rows []chunkJobRow
	err := s.db.WithContext(ctx).Raw(`SELECT j.* FROM t_document_chunk_job j JOIN t_knowledge_document d ON d.id=j.document_id WHERE j.state='pending' AND j.refreshed=false AND d.deleted=0 AND d.enabled=1 AND d.current_chunk_job_id=j.id AND d.chunk_epoch=j.epoch AND d.status='running' ORDER BY j.created_at,j.id LIMIT ?`, limit).Scan(&rows).Error
	result := make([]port.ChunkDocumentTask, 0, len(rows))
	for _, r := range rows {
		result = append(result, port.ChunkDocumentTask{TaskID: r.ID, DocumentID: r.DocumentID, TriggeredBy: r.TriggeredBy})
	}
	return result, err
}
func (s *ChunkJobs) RecoverExpired(ctx context.Context) (int64, error) {
	var rows []chunkJobRow
	err := s.db.WithContext(ctx).Raw(`SELECT j.* FROM t_document_chunk_job j JOIN t_knowledge_document d ON d.id=j.document_id WHERE (j.state='running' AND j.lease_until<=CURRENT_TIMESTAMP) OR (j.state='pending' AND j.refreshed=true AND j.created_at<CURRENT_TIMESTAMP-INTERVAL '3 minutes') OR (j.state IN ('running','pending') AND (d.deleted<>0 OR d.enabled=0 OR d.status<>'running' OR d.current_chunk_job_id IS DISTINCT FROM j.id OR d.chunk_epoch<>j.epoch)) ORDER BY j.created_at LIMIT 100`).Scan(&rows).Error
	if err != nil {
		return 0, err
	}
	var count int64
	for _, row := range rows {
		err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Deleted documents still exist as soft-deleted rows.
			var id string
			if err := tx.Raw(`SELECT id FROM t_knowledge_document WHERE id=? FOR UPDATE`, row.DocumentID).Scan(&id).Error; err != nil {
				return err
			}
			var expired string
			if err := tx.Raw(`SELECT j.id FROM t_document_chunk_job j JOIN t_knowledge_document d ON d.id=j.document_id WHERE j.id=? AND ((j.state='running' AND j.lease_until<=clock_timestamp()) OR (j.state='pending' AND j.refreshed=true AND j.created_at<clock_timestamp()-INTERVAL '3 minutes') OR (j.state IN ('pending','running') AND (d.deleted<>0 OR d.enabled=0 OR d.status<>'running' OR d.current_chunk_job_id IS DISTINCT FROM j.id OR d.chunk_epoch<>j.epoch))) FOR UPDATE OF j`, row.ID).Scan(&expired).Error; err != nil {
				return err
			}
			if expired == "" {
				return nil
			}
			if err := interruptChunkJob(tx, row.ID, row.DocumentID, "interrupted", "execution lease expired or document revoked"); err != nil {
				return err
			}
			count++
			return nil
		})
		if err != nil {
			return count, err
		}
	}
	return count, nil
}
