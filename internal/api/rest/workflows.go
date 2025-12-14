package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GET /api/v1/workflows
func (s *Server) listWorkflows(c *gin.Context) {
	// TODO: Implement workflow listing from DB
	c.JSON(http.StatusOK, gin.H{
		"workflows": []gin.H{},
		"count": 0,
	})
}

// GET /api/v1/workflows/:id
func (s *Server) getWorkflow(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Not implemented yet",
	})
}

// POST /api/v1/workflows
func (s *Server) createWorkflow(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Not implemented yet",
	})
}

// PUT /api/v1/workflows/:id
func (s *Server) updateWorkflow(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Not implemented yet",
	})
}

// DELETE /api/v1/workflows/:id
func (s *Server) deleteWorkflow(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Not implemented yet",
	})
}

// POST /api/v1/workflows/:id/activate
func (s *Server) activateWorkflow(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Not implemented yet",
	})
}
