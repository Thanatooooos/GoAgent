package knowledge

import "github.com/gin-gonic/gin"

func RegisterKnowledgeDocumentRoutes(r gin.IRoutes, service KnowledgeDocumentService) {
	handler := NewKnowledgeDocumentHandler(service)
	r.POST("/knowledge-base/:kb-id/docs/upload", handler.Upload)
	r.POST("/knowledge-base/docs/:docId/chunk", handler.StartChunk)
	r.DELETE("/knowledge-base/docs/:docId", handler.Delete)
	r.GET("/knowledge-base/docs/:docId", handler.Get)
	r.PUT("/knowledge-base/docs/:docId", handler.Update)
	r.GET("/knowledge-base/:kb-id/docs", handler.Page)
	r.GET("/knowledge-base/docs/search", handler.Search)
	r.PATCH("/knowledge-base/docs/:docId/enable", handler.Enable)
	r.GET("/knowledge-base/docs/:docId/chunk-logs", handler.ChunkLogs)
	r.GET("/knowledge-base/docs/:docId/schedule-execs", handler.ScheduleExecs)
}
