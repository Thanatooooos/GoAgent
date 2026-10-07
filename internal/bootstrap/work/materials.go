package work

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/app/work/domain"
	knowledgebootstrap "local/rag-project/internal/bootstrap/knowledge"
	"time"
)

func (r *Runtime) ConfigureMaterials(k *knowledgebootstrap.Runtime, embedding string) {
	r.Knowledge = k
	r.EmbeddingModel = embedding
}
func (r *Runtime) UploadSource(ctx context.Context, user, topic string, reserve domain.ReserveSource, in knowledgeservice.UploadKnowledgeDocumentInput) (domain.Source, error) {
	if r.Knowledge == nil {
		return domain.Source{}, fmt.Errorf("资料服务不可用")
	}
	src, err := r.Store.ReserveSource(ctx, user, topic, r.EmbeddingModel, reserve)
	if err != nil {
		return src, err
	}
	src, err = r.Store.GetSource(ctx, user, topic, src.ID, reserve.ConversationID)
	if err != nil {
		return src, err
	}
	if src.Status == "ready" {
		return r.Store.GetSource(ctx, user, topic, src.ID, reserve.ConversationID)
	}
	claimed, err := r.Store.ClaimSource(ctx, user, topic, src.ID)
	if err != nil {
		return src, err
	}
	if !claimed {
		return src, fmt.Errorf("资料正在上传，请稍后刷新")
	}
	in.KnowledgeBaseID = src.KnowledgeBaseID
	in.OperatorID = user
	in.ScheduleEnabled = false
	in.ProcessMode = "chunk"
	var existing struct{ ID, FileURL string }
	err = r.Knowledge.DB.WithContext(ctx).Raw(`SELECT id,file_url FROM t_knowledge_document WHERE kb_id=? AND deleted=0 ORDER BY create_time LIMIT 1`, src.KnowledgeBaseID).Scan(&existing).Error
	if err != nil {
		return src, err
	}
	var document knowledgedomain.KnowledgeDocument
	if existing.ID != "" {
		document, err = r.Knowledge.DocumentService.Get(ctx, knowledgeservice.GetKnowledgeDocumentInput{DocumentID: existing.ID})
	} else {
		document, err = r.Knowledge.DocumentService.Upload(ctx, in)
	}
	if err != nil {
		_ = r.Store.FailSource(context.WithoutCancel(ctx), user, src.ID, err)
		return src, err
	}
	if err = r.Store.AttachSource(ctx, user, topic, src.ID, document.ID, document.FileURL); err != nil {
		_ = r.Knowledge.DB.WithContext(context.WithoutCancel(ctx)).Exec(`UPDATE t_work_source SET doc_id=?,file_key=?,status='cleanup_pending',updated_at=CURRENT_TIMESTAMP WHERE id=?`, document.ID, document.FileURL, src.ID).Error
		return src, err
	}
	if document.Status == "pending" || document.Status == "failed" {
		if err = r.Knowledge.DocumentService.StartChunk(ctx, knowledgeservice.StartChunkKnowledgeDocumentInput{DocumentID: document.ID, OperatorID: user}); err != nil {
			_ = r.Knowledge.DB.WithContext(context.WithoutCancel(ctx)).Exec(`UPDATE t_work_source SET error=? WHERE id=?`, "解析未启动，请重试", src.ID).Error
		}
	}
	return r.Store.GetSource(ctx, user, topic, src.ID, reserve.ConversationID)
}
func (r *Runtime) RetrySource(ctx context.Context, user, topic, id, conversation string) error {
	src, err := r.Store.GetSource(ctx, user, topic, id, conversation)
	if err != nil {
		return err
	}
	t, err := r.Store.GetTopic(ctx, user, topic)
	if err != nil {
		return err
	}
	if t.Status == "archived" {
		return domain.ErrArchived
	}
	if src.DocumentID == "" {
		return fmt.Errorf("请重新上传此资料")
	}
	return r.Knowledge.DocumentService.StartChunk(ctx, knowledgeservice.StartChunkKnowledgeDocumentInput{DocumentID: src.DocumentID, OperatorID: user})
}
func (r *Runtime) StartCleanup() {
	if r.Knowledge == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cleanupCancel = cancel
	r.cleanupWG.Add(1)
	go func() {
		defer r.cleanupWG.Done()
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			r.CleanupOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}
func (r *Runtime) Close() error {
	if r.cleanupCancel != nil {
		r.cleanupCancel()
	}
	r.cleanupWG.Wait()
	return nil
}
func (r *Runtime) CleanupOnce(ctx context.Context) {
	if r.Knowledge == nil {
		return
	}
	var rows []struct{ ID, UserID, DocID, FileKey string }
	if err := r.Knowledge.DB.WithContext(ctx).Raw(`SELECT id,user_id,COALESCE(doc_id,'') AS doc_id,file_key FROM t_work_source WHERE status='cleanup_pending' AND next_cleanup_at<=CURRENT_TIMESTAMP ORDER BY next_cleanup_at LIMIT 10`).Scan(&rows).Error; err != nil {
		return
	}
	for _, row := range rows {
		var err error
		if row.DocID != "" {
			var n int64
			query := r.Knowledge.DB.WithContext(ctx).Raw(`SELECT COUNT(*) FROM t_knowledge_document WHERE id=? AND deleted=0`, row.DocID).Scan(&n)
			err = query.Error
			if err == nil && n > 0 {
				err = r.Knowledge.DocumentService.Delete(ctx, knowledgeservice.DeleteKnowledgeDocumentInput{DocumentID: row.DocID, OperatorID: row.UserID})
			}
		}
		if err == nil && row.FileKey != "" {
			err = r.Knowledge.Storage.Delete(ctx, row.FileKey)
		}
		if err != nil {
			_ = r.Knowledge.DB.WithContext(ctx).Exec(`UPDATE t_work_source SET cleanup_attempts=cleanup_attempts+1,next_cleanup_at=CURRENT_TIMESTAMP+INTERVAL '30 seconds',error=? WHERE id=? AND status='cleanup_pending'`, "清理待重试", row.ID).Error
			continue
		}
		_ = r.Knowledge.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`UPDATE t_knowledge_base SET deleted=1 WHERE id IN (SELECT kb_id FROM t_work_source WHERE id=? AND status='cleanup_pending') AND work_private=true`, row.ID).Error; err != nil {
				return err
			}
			return tx.Exec(`UPDATE t_work_source SET status='cleaned',error='',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='cleanup_pending'`, row.ID).Error
		})
	}
}
