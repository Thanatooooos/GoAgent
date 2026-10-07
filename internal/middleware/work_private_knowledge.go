package middleware

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"strings"
)

// Private Work sources have their own owner/topic/conversation routes. Global
// knowledge endpoints must not expose them, even to an administrator.
func RejectWorkPrivateKnowledge(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.FullPath(), "/knowledge-base") && !strings.Contains(c.FullPath(), "/knowledge-base/") {
			c.Next()
			return
		}
		var kb string
		if id := c.Param("kb-id"); id != "" {
			kb = id
		}
		if id := c.Param("docId"); id != "" {
			if err := db.WithContext(c.Request.Context()).Raw(`SELECT kb_id FROM t_knowledge_document WHERE id=?`, id).Scan(&kb).Error; err != nil {
				c.AbortWithStatus(500)
				return
			}
		}
		if id := c.Param("chunkId"); id != "" {
			if err := db.WithContext(c.Request.Context()).Raw(`SELECT kb_id FROM t_knowledge_chunk WHERE id=?`, id).Scan(&kb).Error; err != nil {
				c.AbortWithStatus(500)
				return
			}
		}
		if id := c.Param("evidenceId"); id != "" {
			if err := db.WithContext(c.Request.Context()).Raw(`SELECT d.kb_id FROM t_knowledge_image_evidence e JOIN t_knowledge_image_occurrence i ON i.id=e.occurrence_id JOIN t_knowledge_document d ON d.id=i.doc_id WHERE e.id=?`, id).Scan(&kb).Error; err != nil {
				c.AbortWithStatus(500)
				return
			}
		}
		if id := c.Param("occurrenceId"); id != "" {
			if err := db.WithContext(c.Request.Context()).Raw(`SELECT d.kb_id FROM t_knowledge_image_occurrence i JOIN t_knowledge_document d ON d.id=i.doc_id WHERE i.id=?`, id).Scan(&kb).Error; err != nil {
				c.AbortWithStatus(500)
				return
			}
		}
		if kb != "" {
			var private bool
			if err := db.WithContext(c.Request.Context()).Raw(`SELECT work_private FROM t_knowledge_base WHERE id=?`, kb).Scan(&private).Error; err != nil {
				c.AbortWithStatus(500)
				return
			}
			if private {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"code": "WORK_PRIVATE_SOURCE", "message": "资料需通过所属 Work 专题访问"})
				return
			}
		}
		c.Next()
	}
}
