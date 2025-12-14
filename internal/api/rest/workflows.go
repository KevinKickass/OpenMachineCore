package rest

import (
    "encoding/json"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/KevinKickass/OpenMachineCore/internal/storage"
    "github.com/KevinKickass/OpenMachineCore/internal/types"
    "github.com/KevinKickass/OpenMachineCore/internal/workflow/definition"
    "go.uber.org/zap"
)

// GET /api/v1/workflows
func (s *Server) listWorkflows(c *gin.Context) {
    ctx := c.Request.Context()

    workflows, err := s.lm.Storage().ListWorkflows(ctx)
    if err != nil {
        s.logger.Error("Failed to list workflows", zap.Error(err))
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to list workflows",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "workflows": workflows,
        "count":     len(workflows),
    })
}

// GET /api/v1/workflows/:id
func (s *Server) getWorkflow(c *gin.Context) {
    ctx := c.Request.Context()
    
    workflowID, err := uuid.Parse(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid workflow ID",
        })
        return
    }

    workflow, compositions, err := s.lm.Storage().LoadWorkflow(ctx, workflowID)
    if err != nil {
        s.logger.Error("Failed to load workflow", 
            zap.String("workflow_id", workflowID.String()),
            zap.Error(err))
        c.JSON(http.StatusNotFound, gin.H{
            "error": "Workflow not found",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "workflow":     workflow,
        "compositions": compositions,
    })
}

// POST /api/v1/workflows
func (s *Server) createWorkflow(c *gin.Context) {
    ctx := c.Request.Context()

    var req struct {
        WorkflowName string                      `json:"workflow_name" binding:"required"`
        Definition   json.RawMessage             `json:"definition" binding:"required"`
        Compositions []types.DeviceComposition   `json:"compositions"`
        Active       bool                        `json:"active"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid request body",
            "details": err.Error(),
        })
        return
    }

    // Validate workflow definition
    _, err := definition.ParseWorkflow(req.Definition)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid workflow definition",
            "details": err.Error(),
        })
        return
    }

    workflow := &storage.Workflow{
        WorkflowName: req.WorkflowName,
        Definition:   req.Definition,
        Active:       req.Active,
    }

    if err := s.lm.Storage().SaveWorkflow(ctx, workflow, req.Compositions); err != nil {
        s.logger.Error("Failed to create workflow", zap.Error(err))
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to create workflow",
        })
        return
    }

    s.logger.Info("Workflow created", 
        zap.String("workflow_id", workflow.ID.String()),
        zap.String("workflow_name", workflow.WorkflowName))

    c.JSON(http.StatusCreated, gin.H{
        "workflow_id": workflow.ID.String(),
        "message":     "Workflow created successfully",
    })
}

// PUT /api/v1/workflows/:id
func (s *Server) updateWorkflow(c *gin.Context) {
    ctx := c.Request.Context()

    workflowID, err := uuid.Parse(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid workflow ID",
        })
        return
    }

    var req struct {
        WorkflowName string          `json:"workflow_name"`
        Definition   json.RawMessage `json:"definition"`
        Active       *bool           `json:"active"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid request body",
        })
        return
    }

    // Load existing workflow
    workflow, _, err := s.lm.Storage().LoadWorkflow(ctx, workflowID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{
            "error": "Workflow not found",
        })
        return
    }

    // Update fields
    if req.WorkflowName != "" {
        workflow.WorkflowName = req.WorkflowName
    }
    if req.Definition != nil {
        // Validate new definition
        if _, err := definition.ParseWorkflow(req.Definition); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{
                "error": "Invalid workflow definition",
                "details": err.Error(),
            })
            return
        }
        workflow.Definition = req.Definition
    }
    if req.Active != nil {
        workflow.Active = *req.Active
    }

    if err := s.lm.Storage().UpdateWorkflow(ctx, workflow); err != nil {
        s.logger.Error("Failed to update workflow", zap.Error(err))
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to update workflow",
        })
        return
    }

    s.logger.Info("Workflow updated", zap.String("workflow_id", workflowID.String()))

    c.JSON(http.StatusOK, gin.H{
        "message": "Workflow updated successfully",
    })
}

// DELETE /api/v1/workflows/:id
func (s *Server) deleteWorkflow(c *gin.Context) {
    ctx := c.Request.Context()

    workflowID, err := uuid.Parse(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid workflow ID",
        })
        return
    }

    if err := s.lm.Storage().DeleteWorkflow(ctx, workflowID); err != nil {
        s.logger.Error("Failed to delete workflow", zap.Error(err))
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to delete workflow",
        })
        return
    }

    s.logger.Info("Workflow deleted", zap.String("workflow_id", workflowID.String()))

    c.JSON(http.StatusOK, gin.H{
        "message": "Workflow deleted successfully",
    })
}

// POST /api/v1/workflows/:id/activate
func (s *Server) activateWorkflow(c *gin.Context) {
    ctx := c.Request.Context()

    workflowID, err := uuid.Parse(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid workflow ID",
        })
        return
    }

    if err := s.lm.Storage().ActivateWorkflow(ctx, workflowID); err != nil {
        s.logger.Error("Failed to activate workflow", zap.Error(err))
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to activate workflow",
        })
        return
    }

    s.logger.Info("Workflow activated", zap.String("workflow_id", workflowID.String()))

    c.JSON(http.StatusOK, gin.H{
        "message": "Workflow activated successfully",
    })
}

// POST /api/v1/workflows/:id/execute
func (s *Server) executeWorkflow(c *gin.Context) {
    ctx := c.Request.Context()

    workflowID, err := uuid.Parse(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid workflow ID",
        })
        return
    }

    var input map[string]interface{}
    if err := c.ShouldBindJSON(&input); err != nil {
        // If no body or invalid JSON, use empty input
        input = make(map[string]interface{})
    }

    executionID, err := s.lm.WorkflowEngine().ExecuteWorkflow(ctx, workflowID, input)
    if err != nil {
        s.logger.Error("Failed to execute workflow", 
            zap.String("workflow_id", workflowID.String()),
            zap.Error(err))
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to execute workflow",
            "details": err.Error(),
        })
        return
    }

    s.logger.Info("Workflow execution started", 
        zap.String("workflow_id", workflowID.String()),
        zap.String("execution_id", executionID.String()))

    c.JSON(http.StatusAccepted, gin.H{
        "execution_id": executionID.String(),
        "message":      "Workflow execution started",
    })
}

// GET /api/v1/executions/:id
func (s *Server) getExecutionStatus(c *gin.Context) {
    ctx := c.Request.Context()

    executionID, err := uuid.Parse(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid execution ID",
        })
        return
    }

    exec, steps, err := s.lm.WorkflowEngine().GetExecutionStatus(ctx, executionID)
    if err != nil {
        s.logger.Error("Failed to get execution status", zap.Error(err))
        c.JSON(http.StatusNotFound, gin.H{
            "error": "Execution not found",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "execution": exec,
        "steps":     steps,
    })
}

// GET /api/v1/executions/:id/steps
func (s *Server) getExecutionSteps(c *gin.Context) {
    ctx := c.Request.Context()

    executionID, err := uuid.Parse(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid execution ID",
        })
        return
    }

    steps, err := s.lm.Storage().GetExecutionSteps(ctx, executionID)
    if err != nil {
        s.logger.Error("Failed to get execution steps", zap.Error(err))
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to get execution steps",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "steps": steps,
        "count": len(steps),
    })
}
