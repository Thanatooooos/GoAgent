package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	raghttp "local/rag-project/internal/adapter/http/rag"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/runtime/persistence"
	"local/rag-project/internal/app/work/domain"
	workbootstrap "local/rag-project/internal/bootstrap/work"
	"local/rag-project/internal/middleware"
)

func RegisterChatRoutes(r gin.IRouter, runtime *workbootstrap.Runtime) {
	h := &chatHandler{runtime: runtime}
	g := r.Group("/work/topics/:topicId")
	g.Use(middleware.RequireLogin())
	g.POST("/chat", h.chat)
	g.POST("/intent", func(c *gin.Context) {
		in, ok := bind[struct {
			Question   string `json:"question"`
			ArtifactID string `json:"artifactId"`
		}](c)
		if !ok {
			return
		}
		v, e := runtime.ResolveIntent(c.Request.Context(), uid(c), c.Param("topicId"), in.Question, in.ArtifactID)
		reply(c, v, e)
	})
	g.GET("/turns/:turnId", h.turn)
	g.GET("/turns/:turnId/document-draft", func(c *gin.Context) {
		v, e := runtime.Store.ReadDocumentDraft(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("turnId"))
		reply(c, v, e)
	})
	g.GET("/turns", func(c *gin.Context) {
		p, ok := page(c)
		if !ok {
			return
		}
		v, e := runtime.Store.ListTurns(c.Request.Context(), uid(c), c.Param("topicId"), c.Query("conversationId"), p)
		reply(c, v, e)
	})
	g.GET("/turns/:turnId/stream", h.replay)
	g.POST("/turns/:turnId/stop", h.stop)
	g.GET("/conversations/:conversationId/messages", h.messages)
	g.GET("/messages/:messageId", func(c *gin.Context) {
		v, e := runtime.Store.GetMessage(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("messageId"))
		reply(c, v, e)
	})
}

type chatHandler struct{ runtime *workbootstrap.Runtime }

func (h *chatHandler) chat(c *gin.Context) {
	if h.runtime.Chat == nil {
		reply(c, nil, fmt.Errorf("Work chat is unavailable"))
		return
	}
	in, ok := bind[domain.ChatRequest](c)
	if !ok {
		return
	}
	user, topic := uid(c), c.Param("topicId")
	turn, err := h.runtime.Store.AcceptTurn(c.Request.Context(), user, topic, in)
	if err != nil {
		reply(c, nil, err)
		return
	}
	started, err := h.runtime.Store.StartTurn(c.Request.Context(), user, topic, turn.ID)
	if err != nil {
		reply(c, nil, err)
		return
	}
	if started {
		_ = raghttp.SendRuntimeMeta(h.runtime.Streams, turn.ID, turn.ConversationID)
		ctx := context.WithoutCancel(c.Request.Context())
		go raghttp.WatchRuntimeStop(ctx, h.runtime.Streams, turn.ID, func() bool { return h.runtime.Chat.CancelTask(turn.ID) })
		go func() {
			ctx, cancel := context.WithDeadline(ctx, turn.DeadlineAt)
			defer cancel()
			snapshot, _ := json.Marshal(turn.Snapshot)
			ids, scopeErr := h.runtime.Store.SourceScope(ctx, user, topic, turn.ConversationID)
			if scopeErr != nil {
				_ = h.runtime.Store.FinishTurn(context.WithoutCancel(ctx), user, topic, turn.ID, "failed", "")
				raghttp.SendRuntimeFailure(h.runtime.Streams, turn.ID, scopeErr)
				return
			}
			result, err := h.runtime.Chat.Chat(ctx, conversationruntime.ChatInput{TaskID: turn.ID, TraceID: turn.ID, UserID: user, KnowledgeBaseIDs: ids, ConversationID: turn.ConversationID, AcceptedUserMessageID: turn.UserMessageID, Question: turn.Question, Work: &capability.WorkScope{TopicID: topic, ItemID: turn.ItemID, TurnID: turn.ID, ArtifactID: turn.ArtifactID, ArtifactRevision: turn.ArtifactRevision, Action: turn.Action, Snapshot: string(snapshot)}, Policy: conversationruntime.Policy{AllowWebSearch: true, AllowKnowledgeRetrieval: len(ids) > 0}}, raghttp.NewRuntimeEventSink(h.runtime.Streams, turn.ID))
			if errors.Is(err, persistence.ErrExecutionLeaseLost) {
				// Recovery owns the terminal outcome; an old worker must not overwrite it.
				_ = raghttp.RecoverRuntimePublication(context.WithoutCancel(ctx), h.runtime.Streams, turn.ID, func(ctx context.Context) (conversationruntime.JournalEntry, error) {
					return h.runtime.Chat.RecoverPublication(ctx, user, turn.ID)
				})
				return
			}
			status := "completed"
			if err != nil {
				status = "failed"
			}
			if result.AssistantMessage.ID != "" {
				status = "completed"
			}
			finishErr := h.runtime.Store.FinishTurn(context.WithoutCancel(ctx), user, topic, turn.ID, status, result.AssistantMessage.ID)
			if err != nil {
				raghttp.SendRuntimeFailure(h.runtime.Streams, turn.ID, err)
			} else if finishErr != nil {
				raghttp.SendRuntimeFailure(h.runtime.Streams, turn.ID, fmt.Errorf("answer saved but turn status could not be saved"))
			}
		}()
	} else {
		if err := h.recoverStream(c, turn); err != nil {
			reply(c, nil, err)
			return
		}
	}
	raghttp.ServeRuntimeStream(c, h.runtime.Streams, turn.ID, 0)
}
func (h *chatHandler) turn(c *gin.Context) {
	v, e := h.runtime.Store.GetTurn(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("turnId"))
	reply(c, v, e)
}
func (h *chatHandler) replay(c *gin.Context) {
	turn, err := h.runtime.Store.GetTurn(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("turnId"))
	if err != nil {
		reply(c, nil, err)
		return
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		reply(c, nil, fmt.Errorf("invalid stream offset"))
		return
	}
	if h.runtime.Chat == nil {
		reply(c, nil, fmt.Errorf("Work chat is unavailable"))
		return
	}
	if err := h.recoverStream(c, turn); err != nil {
		reply(c, nil, err)
		return
	}
	events, next, err := h.runtime.Streams.GetEvents(c.Request.Context(), turn.ID, 0)
	if err != nil {
		reply(c, nil, err)
		return
	}
	offset = raghttp.PublicationResumeOffset(events, offset)
	if len(events) > 0 && events[len(events)-1].Done && offset >= next {
		offset = next - 1
	}
	raghttp.ServeRuntimeStream(c, h.runtime.Streams, turn.ID, offset)
}
func (h *chatHandler) stop(c *gin.Context) {
	err := h.runtime.Store.CancelTurn(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("turnId"))
	if err != nil {
		reply(c, nil, err)
		return
	}
	if h.runtime.Chat != nil {
		h.runtime.Chat.CancelTask(c.Param("turnId"))
	}
	if err := raghttp.StopRuntimeStream(c.Request.Context(), h.runtime.Streams, c.Param("turnId")); err != nil {
		reply(c, nil, err)
		return
	}
	reply(c, gin.H{"stopped": true}, nil)
}

func (h *chatHandler) recoverStream(c *gin.Context, turn domain.Turn) error {
	events, _, err := h.runtime.Streams.GetEvents(c.Request.Context(), turn.ID, 0)
	if err != nil {
		return err
	}
	if len(events) > 0 {
		if err := raghttp.RecoverRuntimePublication(c.Request.Context(), h.runtime.Streams, turn.ID, func(ctx context.Context) (conversationruntime.JournalEntry, error) {
			return h.runtime.Chat.RecoverPublication(ctx, uid(c), turn.ID)
		}); err != nil {
			return err
		}
		events, _, err = h.runtime.Streams.GetEvents(c.Request.Context(), turn.ID, 0)
		if err != nil {
			return err
		}
		for _, e := range events {
			if e.Done {
				return nil
			}
		}
		if turn.Status != "accepted" && turn.Status != "running" {
			return raghttp.EndRuntimeStream(c.Request.Context(), h.runtime.Streams, turn.ID)
		}
		return nil
	}
	if _, err = h.runtime.Chat.Replay(c.Request.Context(), uid(c), turn.ID, raghttp.NewRuntimeEventSink(h.runtime.Streams, turn.ID)); err != nil {
		return err
	}
	events, _, err = h.runtime.Streams.GetEvents(c.Request.Context(), turn.ID, 0)
	if err != nil {
		return err
	}
	if turn.Status != "accepted" && turn.Status != "running" {
		if len(events) == 0 {
			_ = raghttp.SendRuntimeMeta(h.runtime.Streams, turn.ID, turn.ConversationID)
			if turn.Status != "completed" {
				raghttp.SendRuntimeFailure(h.runtime.Streams, turn.ID, fmt.Errorf("执行已结束（%s），已保存内容仍可查看", turn.Status))
				return nil
			}
		}
		for _, e := range events {
			if e.Done {
				return nil
			}
		}
		return raghttp.EndRuntimeStream(c.Request.Context(), h.runtime.Streams, turn.ID)
	}
	return nil
}
func (h *chatHandler) messages(c *gin.Context) {
	p, ok := page(c)
	if !ok {
		return
	}
	v, e := h.runtime.Store.ListMessages(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("conversationId"), p)
	reply(c, v, e)
}
