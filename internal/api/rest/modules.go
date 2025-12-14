package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GET /api/v1/modules
func (s *Server) listModules(c *gin.Context) {
	// TODO: Scan device-descriptors and list available modules
	c.JSON(http.StatusOK, gin.H{
		"modules": gin.H{
			"beckhoff": []gin.H{
				{"id": "bk9100", "type": "coupler", "name": "BK9100"},
				{"id": "kl1408", "type": "input", "name": "KL1408"},
				{"id": "kl2408", "type": "output", "name": "KL2408"},
			},
		},
	})
}

// GET /api/v1/modules/:vendor/:model
func (s *Server) getModule(c *gin.Context) {
	vendor := c.Param("vendor")
	model := c.Param("model")
	
	c.JSON(http.StatusOK, gin.H{
		"vendor": vendor,
		"model": model,
		"message": "Module details - to be implemented",
	})
}
