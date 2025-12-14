package rest

import (
    "net/http"

    "github.com/gin-gonic/gin"
	"github.com/google/uuid"
    "github.com/KevinKickass/OpenMachineCore/internal/machine"
    "go.uber.org/zap"
)

// GET /api/v1/machine/status
func (s *Server) getMachineStatus(c *gin.Context) {
    status := s.lm.MachineController().GetStatus()
    c.JSON(http.StatusOK, status)
}

// POST /api/v1/machine/command
func (s *Server) executeMachineCommand(c *gin.Context) {
    var req struct {
        Command string `json:"command" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    cmd := machine.Command(req.Command)
    
    if err := s.lm.MachineController().ExecuteCommand(c.Request.Context(), cmd); err != nil {
        s.logger.Error("Machine command failed",
            zap.String("command", req.Command),
            zap.Error(err))
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusAccepted, gin.H{
        "message": "Command accepted",
        "command": req.Command,
    })
}

// POST /api/v1/machine/configure
func (s *Server) configureMachineWorkflows(c *gin.Context) {
    var req struct {
        StopWorkflowID       string `json:"stop_workflow_id" binding:"required"`
        HomeWorkflowID       string `json:"home_workflow_id" binding:"required"`
        ProductionWorkflowID string `json:"production_workflow_id" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    stopID, err := uuid.Parse(req.StopWorkflowID)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid stop_workflow_id"})
        return
    }

    homeID, err := uuid.Parse(req.HomeWorkflowID)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid home_workflow_id"})
        return
    }

    productionID, err := uuid.Parse(req.ProductionWorkflowID)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid production_workflow_id"})
        return
    }

    s.lm.MachineController().SetWorkflows(stopID, homeID, productionID)

    c.JSON(http.StatusOK, gin.H{
        "message": "Machine workflows configured",
    })
}
