package main

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/framework/convention"
	infraai "local/rag-project/internal/infra-ai"
)

type debugChatRequest struct {
	Prompt   string `json:"prompt" binding:"required"`
	ModelID  string `json:"modelId"`
	Thinking *bool  `json:"thinking,omitempty"`
}

func registerDebugAIRoutes(r *gin.Engine, runtime *infraai.Runtime) {
	r.GET("/debug/ai/runtime", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"chatClients":      len(runtime.ChatClients),
			"embeddingClients": len(runtime.EmbeddingClients),
			"rerankClients":    len(runtime.RerankClients),
			"chatReady":        runtime.Chat != nil,
			"embeddingReady":   runtime.Embedding != nil,
			"rerankReady":      runtime.Rerank != nil,
		})
	})

	r.POST("/debug/ai/chat", func(c *gin.Context) {
		var req debugChatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid request",
				"error":   err.Error(),
			})
			return
		}

		chatReq := convention.ChatRequest{
			Messages: []convention.ChatMessage{
				convention.UserMessage(req.Prompt),
			},
			Thinking: req.Thinking,
		}

		var (
			content string
			err     error
		)
		if req.ModelID != "" {
			content, err = runtime.Chat.ChatWithModel(chatReq, req.ModelID)
		} else {
			content, err = runtime.Chat.ChatWithRequest(chatReq)
		}
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"message": "chat request failed",
				"error":   err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"content":  content,
			"modelId":  req.ModelID,
			"thinking": chatReq.ThinkingEnabled(),
		})
	})
}
