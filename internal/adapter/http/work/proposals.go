package work

import (
	"github.com/gin-gonic/gin"
	"local/rag-project/internal/app/work/domain"
	workbootstrap "local/rag-project/internal/bootstrap/work"
	"local/rag-project/internal/middleware"
)

func RegisterProposalRoutes(r gin.IRouter, runtime *workbootstrap.Runtime) {
	g := r.Group("/work/topics/:topicId")
	g.Use(middleware.RequireLogin())
	g.GET("/proposals", func(c *gin.Context) {
		p, ok := page(c)
		if !ok {
			return
		}
		v, e := runtime.Store.ListProposals(c.Request.Context(), uid(c), c.Param("topicId"), p)
		reply(c, v, e)
	})
	g.POST("/proposals/:proposalId/resolve", func(c *gin.Context) {
		in, ok := bind[domain.ResolveProposal](c)
		if !ok {
			return
		}
		v, e := runtime.Store.ResolveProposal(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("proposalId"), in)
		reply(c, v, e)
	})
}
