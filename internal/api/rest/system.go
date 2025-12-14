package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GET /api/v1/system/status
func (s *Server) getSystemStatus(c *gin.Context) {
	status := s.lifecycle.GetCurrentStatus()
	c.JSON(http.StatusOK, status)
}

// POST /api/v1/system/update
func (s *Server) triggerUpdate(c *gin.Context) {
	var req struct {
		WorkflowPath string `json:"workflow_path" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	if err := s.lifecycle.TriggerUpdate(req.WorkflowPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Update initiated",
		"status":  "updating",
	})
}

// POST /api/v1/system/shutdown
func (s *Server) shutdown(c *gin.Context) {
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Shutdown initiated",
	})
	
	// Trigger shutdown in background
	go func() {
		ctx := c.Request.Context()
		s.lifecycle.Shutdown(ctx)
	}()
}
