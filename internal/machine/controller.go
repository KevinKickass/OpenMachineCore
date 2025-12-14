package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/storage"
	"github.com/KevinKickass/OpenMachineCore/internal/workflow/engine"
	"github.com/KevinKickass/OpenMachineCore/internal/api/websocket"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Controller struct {
	logger         *zap.Logger
	workflowEngine *engine.Engine
	storage        *storage.PostgresClient
	wsHub           *websocket.Hub

	mu               sync.RWMutex
	currentState     State
	currentExecID    uuid.UUID
	productionCycles int
	errorMessage     string

	// Workflow IDs für verschiedene Abläufe
	stopWorkflowID       uuid.UUID
	homeWorkflowID       uuid.UUID
	productionWorkflowID uuid.UUID
}

func NewController(
	logger *zap.Logger,
	workflowEngine *engine.Engine,
	storage *storage.PostgresClient,
	wsHub *websocket.Hub,
) *Controller {
	return &Controller{
		wsHub:          wsHub,
		logger:         logger,
		workflowEngine: workflowEngine,
		storage:        storage,
		currentState:   StateStopped,
	}
}

// SetWorkflows configures the workflow IDs for machine operations
func (c *Controller) SetWorkflows(stopID, homeID, productionID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stopWorkflowID = stopID
	c.homeWorkflowID = homeID
	c.productionWorkflowID = productionID

	c.logger.Info("Machine workflows configured",
		zap.String("stop", stopID.String()),
		zap.String("home", homeID.String()),
		zap.String("production", productionID.String()))
}

// ExecuteCommand handles machine commands
func (c *Controller) ExecuteCommand(ctx context.Context, cmd Command) error {
	c.mu.Lock()
	currentState := c.currentState
	c.mu.Unlock()

	c.logger.Info("Machine command received",
		zap.String("command", string(cmd)),
		zap.String("current_state", string(currentState)))

	switch cmd {
	case CommandHome:
		return c.executeHome(ctx)
	case CommandStart:
		return c.executeStart(ctx)
	case CommandStop:
		return c.executeStop(ctx)
	case CommandReset:
		return c.executeReset(ctx)
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func (c *Controller) executeHome(ctx context.Context) error {
	c.mu.Lock()
	if c.currentState != StateStopped {
		c.mu.Unlock()
		return fmt.Errorf("cannot home: machine must be stopped (current: %s)", c.currentState)
	}
	c.currentState = StateHoming
	c.mu.Unlock()

	// Execute homing workflow
	execID, err := c.workflowEngine.ExecuteWorkflow(ctx, c.homeWorkflowID, nil)
	if err != nil {
		c.setState(StateError, err.Error())
		return err
	}

	c.mu.Lock()
	c.currentExecID = execID
	c.mu.Unlock()

	// Monitor workflow completion (in background)
	go c.monitorWorkflow(execID, StateReady)

	return nil
}

func (c *Controller) executeStart(ctx context.Context) error {
	c.mu.Lock()
	if c.currentState != StateReady {
		c.mu.Unlock()
		return fmt.Errorf("cannot start: machine must be ready (current: %s)", c.currentState)
	}
	c.currentState = StateRunning
	c.productionCycles = 0
	c.mu.Unlock()

	// Execute production workflow (with continuous loop)
	execID, err := c.workflowEngine.ExecuteWorkflow(ctx, c.productionWorkflowID, nil)
	if err != nil {
		c.setState(StateError, err.Error())
		return err
	}

	c.mu.Lock()
	c.currentExecID = execID
	c.mu.Unlock()

	// Monitor workflow for errors
	go c.monitorProductionWorkflow(execID)

	return nil
}

func (c *Controller) executeStop(ctx context.Context) error {
	c.mu.Lock()
	if c.currentState != StateRunning {
		c.mu.Unlock()
		return fmt.Errorf("cannot stop: machine not running (current: %s)", c.currentState)
	}

	// Cancel running production workflow
	if c.currentExecID != uuid.Nil {
		c.workflowEngine.CancelExecution(ctx, c.currentExecID)
	}

	c.currentState = StateStopping
	c.mu.Unlock()

	// Execute stop workflow
	execID, err := c.workflowEngine.ExecuteWorkflow(ctx, c.stopWorkflowID, nil)
	if err != nil {
		c.setState(StateError, err.Error())
		return err
	}

	c.mu.Lock()
	c.currentExecID = execID
	c.mu.Unlock()

	// Monitor workflow completion
	go c.monitorWorkflow(execID, StateStopped)

	return nil
}

func (c *Controller) executeReset(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.currentState != StateError && c.currentState != StateEmergency {
		return fmt.Errorf("cannot reset: no error state (current: %s)", c.currentState)
	}

	c.currentState = StateStopped
	c.errorMessage = ""
	c.currentExecID = uuid.Nil

	c.logger.Info("Machine reset to stopped state")
	return nil
}

func (c *Controller) monitorWorkflow(execID uuid.UUID, targetState State) {
	// Poll workflow status
	ctx := context.Background()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		exec, _, err := c.workflowEngine.GetExecutionStatus(ctx, execID)
		if err != nil {
			c.logger.Error("Failed to get execution status", zap.Error(err))
			continue
		}

		switch exec.Status {
		case storage.StatusSuccess:
			c.setState(targetState, "")
			c.logger.Info("Workflow completed successfully",
				zap.String("execution_id", execID.String()),
				zap.String("new_state", string(targetState)))
			return

		case storage.StatusFailed:
			c.setState(StateError, exec.Error)
			c.logger.Error("Workflow failed",
				zap.String("execution_id", execID.String()),
				zap.String("error", exec.Error))
			return

		case storage.StatusCancelled:
			// Expected for stop command
			return
		}
	}
}

func (c *Controller) monitorProductionWorkflow(execID uuid.UUID) {
	// Monitor for errors during production
	ctx := context.Background()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.RLock()
		state := c.currentState
		c.mu.RUnlock()

		if state != StateRunning {
			return
		}

		exec, _, err := c.workflowEngine.GetExecutionStatus(ctx, execID)
		if err != nil {
			continue
		}

		// Count completed cycles from output
		if exec.Output != nil {
			var output map[string]interface{}
			json.Unmarshal(exec.Output, &output)
			if cycles, ok := output["iterations_completed"].(float64); ok {
				c.mu.Lock()
				c.productionCycles = int(cycles)
				c.mu.Unlock()
			}
		}

		if exec.Status == storage.StatusFailed {
			c.setState(StateError, exec.Error)
			return
		}
	}
}

func (c *Controller) setState(state State, errorMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	previousState := c.currentState
	c.currentState = state
	c.errorMessage = errorMsg

	c.logger.Info("Machine state changed",
		zap.String("state", string(state)),
		zap.String("error", errorMsg))

	// Broadcast state change via WebSocket
	if c.wsHub != nil {
		c.wsHub.Broadcast(websocket.NewMachineStateMessage(
			string(state), 
			string(previousState),
		))
	}
}

func (c *Controller) GetStatus() MachineStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return MachineStatus{
		State:            c.currentState,
		ExecutionID:      c.currentExecID.String(),
		ErrorMessage:     c.errorMessage,
		ProductionCycles: c.productionCycles,
		LastStateChange:  time.Now(),
	}
}
