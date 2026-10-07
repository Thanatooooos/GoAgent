package process

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/distributedid"
)

func (s *DocumentProcessService) FencedChunkProcessing() bool { return s != nil && s.jobs != nil }
func (s *DocumentProcessService) executeOwnedChunk(ctx context.Context, input ExecuteChunkInput) (err error) {
	if err := s.validateDependencies(); err != nil {
		return err
	}
	if s.transaction == nil {
		return fmt.Errorf("owned chunk publication requires a transaction")
	}
	if input.TaskID == "" {
		id, err := distributedid.NextID()
		if err != nil {
			return err
		}
		input.TaskID = strconv.FormatInt(id, 10)
		if input.TriggeredBy == "" {
			input.TriggeredBy = "system"
		}
		if _, err := s.jobs.Admit(ctx, port.ChunkDocumentTask{TaskID: input.TaskID, DocumentID: input.DocumentID, TriggeredBy: input.TriggeredBy}, nil); err != nil {
			return err
		}
	}
	ttl := s.leaseDuration
	if ttl <= 0 {
		ttl = 3 * time.Minute
	}
	job, err := s.jobs.Claim(ctx, input.TaskID, uuid.NewString(), ttl)
	if err != nil || job == nil {
		return err
	} // duplicate delivery cannot execute twice
	ctx, cancel := context.WithCancel(port.WithChunkJob(ctx, *job))
	defer cancel()
	input.DocumentID = job.DocumentID
	input.TriggeredBy = job.TriggeredBy
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(ttl / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.jobs.Renew(ctx, *job, ttl); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-heartbeatDone
		if err != nil {
			cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer stop()
			_ = s.jobs.Fail(cleanup, *job, truncateChunkLogError(chunkLogError(err)))
		}
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("chunk execution panic: %v", recovered)
		}
	}()
	return s.executeChunk(ctx, input)
}

func (s *DocumentProcessService) processOwnedRefresh(ctx context.Context, document domain.KnowledgeDocument) error {
	id, err := distributedid.NextID()
	if err != nil {
		return err
	}
	task := port.ChunkDocumentTask{TaskID: strconv.FormatInt(id, 10), DocumentID: document.ID, TriggeredBy: "system"}
	if _, err := s.jobs.Admit(ctx, task, &document); err != nil {
		return err
	}
	return s.executeOwnedChunk(ctx, ExecuteChunkInput{TaskID: task.TaskID, DocumentID: task.DocumentID, TriggeredBy: task.TriggeredBy})
}

func (s *DocumentProcessService) completeOwnedPublication(ctx context.Context, repo port.KnowledgeDocumentRepository, result documentProcessResult) error {
	job, ok := port.CurrentChunkJob(ctx)
	if !ok {
		return nil
	}
	finalizer, ok := repo.(interface {
		CompleteChunkJob(context.Context, domain.KnowledgeDocumentChunkLog, string) error
	})
	if !ok {
		return fmt.Errorf("chunk publication fencing is unavailable")
	}
	end := s.now()
	log := domain.NewKnowledgeDocumentChunkLog(job.TaskID, job.DocumentID)
	log.Status = domain.KnowledgeDocumentChunkLogStatusSuccess
	log.ProcessMode = job.Document.ProcessMode
	log.ChunkStrategy = job.Document.ChunkStrategy
	log.EndTime = &end
	log.UpdatedAt = end
	log.ExtractDuration = result.ExtractDuration
	log.ChunkDuration = result.ChunkDuration
	log.EmbedDuration = result.EmbedDuration
	log.PersistDuration = result.PersistDuration
	log.TotalDuration = result.TotalDuration
	log.ChunkCount = result.ChunkCount
	status := domain.KnowledgeDocumentStatusSuccess
	if result.ImagePending {
		status = domain.KnowledgeDocumentStatusRunning
	} else if result.ImagePartial {
		status = domain.KnowledgeDocumentStatusPartial
		if result.ImageOnly {
			status = domain.KnowledgeDocumentStatusFailed
		}
	}
	return finalizer.CompleteChunkJob(ctx, log, status)
}
