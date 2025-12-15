package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/api/websocket"
	"github.com/KevinKickass/OpenMachineCore/internal/storage"
	"github.com/KevinKickass/OpenMachineCore/internal/workflow/definition"
	"github.com/KevinKickass/OpenMachineCore/internal/workflow/executor"
	"github.com/KevinKickass/OpenMachineCore/internal/workflow/streaming"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ExecutionTracker maintains call stack and hierarchical step information for a running workflow
type ExecutionTracker struct {
	ExecutionID uuid.UUID
	CallStack   []definition.CallFrame // Stack of (workflow_id, program_name, step_number)
	mu          sync.RWMutex
}

// NewExecutionTracker creates a new execution tracker
func NewExecutionTracker(executionID uuid.UUID) *ExecutionTracker {
	return &ExecutionTracker{
		ExecutionID: executionID,
		CallStack:   make([]definition.CallFrame, 0),
	}
}

// Push adds a new level to the call stack when entering a subroutine
func (et *ExecutionTracker) Push(workflowID string, programName string, stepNumber string) {
	et.mu.Lock()
	defer et.mu.Unlock()
	et.CallStack = append(et.CallStack, definition.CallFrame{
		WorkflowID:  workflowID,
		ProgramName: programName,
		StepNumber:  stepNumber,
	})
}

// Pop removes a level from the call stack when exiting a subroutine
func (et *ExecutionTracker) Pop() {
	et.mu.Lock()
	defer et.mu.Unlock()
	if len(et.CallStack) > 0 {
		et.CallStack = et.CallStack[:len(et.CallStack)-1]
	}
}

// SetCurrentStep updates the top of the call stack with the current step number
func (et *ExecutionTracker) SetCurrentStep(stepNumber string) {
	et.mu.Lock()
	defer et.mu.Unlock()
	if len(et.CallStack) > 0 {
		et.CallStack[len(et.CallStack)-1].StepNumber = stepNumber
	}
}

// GetHierarchicalStepID returns the full hierarchical step ID
func (et *ExecutionTracker) GetHierarchicalStepID() string {
	et.mu.RLock()
	defer et.mu.RUnlock()
	return definition.BuildHierarchicalStepID(et.CallStack)
}

// GetCallStackCopy returns a copy of the current call stack
func (et *ExecutionTracker) GetCallStackCopy() []definition.CallFrame {
	et.mu.RLock()
	defer et.mu.RUnlock()
	callStackCopy := make([]definition.CallFrame, len(et.CallStack))
	copy(callStackCopy, et.CallStack)
	return callStackCopy
}

// GetDepth returns the current nesting depth
func (et *ExecutionTracker) GetDepth() int {
	et.mu.RLock()
	defer et.mu.RUnlock()
	return len(et.CallStack)
}

type Engine struct {
	storage  *storage.PostgresClient
	executor *executor.StepExecutor
	streamer *streaming.EventStreamer
	logger   *zap.Logger
	wsHub    *websocket.Hub

	runningMu         sync.RWMutex
	runningContexts   map[uuid.UUID]context.CancelFunc
	executionTrackers map[uuid.UUID]*ExecutionTracker // Track call stacks per execution
}

func NewEngine(storage *storage.PostgresClient, executor *executor.StepExecutor, streamer *streaming.EventStreamer, logger *zap.Logger, wsHub *websocket.Hub) *Engine {
	return &Engine{
		storage:           storage,
		executor:          executor,
		streamer:          streamer,
		runningContexts:   make(map[uuid.UUID]context.CancelFunc),
		executionTrackers: make(map[uuid.UUID]*ExecutionTracker),
		logger:            logger,
		wsHub:             wsHub,
	}
}

func (e *Engine) ExecuteWorkflow(ctx context.Context, workflowID uuid.UUID, input map[string]any) (uuid.UUID, error) {
	// Load workflow definition
	workflow, _, err := e.storage.LoadWorkflow(ctx, workflowID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to load workflow: %w", err)
	}

	// Parse workflow definition JSON
	workflowDef, err := definition.ParseWorkflow(workflow.Definition)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to parse workflow definition: %w", err)
	}

	// Create execution record
	executionID := uuid.New()
	inputJSON, _ := json.Marshal(input)

	exec := &storage.WorkflowExecution{
		ID:         executionID,
		WorkflowID: workflowID,
		Status:     storage.StatusPending,
		Input:      inputJSON,
		StartedAt:  time.Now(),
	}

	if err := e.storage.CreateExecution(ctx, exec); err != nil {
		return uuid.Nil, fmt.Errorf("failed to create execution: %w", err)
	}

	// Broadcast workflow started event
	if e.wsHub != nil {
		e.wsHub.Broadcast(websocket.NewWorkflowMessage(
			websocket.MessageTypeWorkflowStarted,
			executionID.String(),
			workflowID.String(),
			"",
			string(storage.StatusPending),
			"",
		))
	}

	// Create cancellable context for this execution
	execCtx, cancel := context.WithCancel(context.Background())

	// Create execution tracker for hierarchical step tracking
	tracker := NewExecutionTracker(executionID)
	// Push the root workflow onto the call stack
	tracker.Push(workflowID.String(), workflowDef.ProgramName, "0")

	e.runningMu.Lock()
	e.runningContexts[executionID] = cancel
	e.executionTrackers[executionID] = tracker
	e.runningMu.Unlock()

	// Execute asynchronously
	go func() {
		defer func() {
			e.runningMu.Lock()
			delete(e.runningContexts, executionID)
			delete(e.executionTrackers, executionID)
			e.runningMu.Unlock()
		}()
		e.runExecution(execCtx, exec, workflowDef, input)
	}()

	return executionID, nil
}

// CancelExecution stops a running workflow execution
func (e *Engine) CancelExecution(ctx context.Context, executionID uuid.UUID) error {
	e.runningMu.RLock()
	cancel, exists := e.runningContexts[executionID]
	e.runningMu.RUnlock()

	if !exists {
		return fmt.Errorf("execution not found or not running: %s", executionID)
	}

	cancel()
	return nil
}

func (e *Engine) cancelExecution(ctx context.Context, exec *storage.WorkflowExecution) {
	now := time.Now()
	exec.Status = storage.StatusCancelled
	exec.CompletedAt = &now
	e.storage.UpdateExecution(ctx, exec)
	e.publishEvent(ctx, exec.ID, "execution.cancelled", nil)
}

func (e *Engine) runExecution(ctx context.Context, exec *storage.WorkflowExecution, workflowDef *definition.Workflow, input map[string]any) {
	// Get tracker for this execution
	e.runningMu.RLock()
	tracker, _ := e.executionTrackers[exec.ID]
	e.runningMu.RUnlock()

	// Update status to running
	exec.Status = storage.StatusRunning
	e.storage.UpdateExecution(ctx, exec)

	// Broadcast running status
	if e.wsHub != nil {
		e.wsHub.Broadcast(websocket.NewWorkflowMessage(
			websocket.MessageTypeWorkflowStep,
			exec.ID.String(),
			exec.WorkflowID.String(),
			"",
			string(storage.StatusRunning),
			"Workflow execution started",
		))
	}

	// Execute steps
	for i, step := range workflowDef.Steps {
		select {
		case <-ctx.Done():
			// Execution cancelled
			exec.Status = storage.StatusCancelled
			now := time.Now()
			exec.CompletedAt = &now

			if tracker != nil {
				exec.CurrentStepID = tracker.GetHierarchicalStepID()
				callStack := tracker.GetCallStackCopy()
				if callStackJSON, err := json.Marshal(callStack); err == nil {
					exec.CallStack = callStackJSON
				}
			}

			e.storage.UpdateExecution(ctx, exec)

			if e.wsHub != nil {
				e.wsHub.Broadcast(websocket.NewWorkflowMessage(
					websocket.MessageTypeWorkflowCancelled,
					exec.ID.String(),
					exec.WorkflowID.String(),
					step.Name,
					string(storage.StatusCancelled),
					"Workflow execution cancelled",
				))
			}
			return

		default:
			// Broadcast step start
			if e.wsHub != nil {
				e.wsHub.Broadcast(websocket.NewWorkflowMessage(
					websocket.MessageTypeWorkflowStep,
					exec.ID.String(),
					exec.WorkflowID.String(),
					step.Name,
					"running",
					fmt.Sprintf("Executing step: %s", step.Name),
				))
			}

			// Execute step with correct parameters
			_, err := e.executeStep(ctx, exec.ID, i, &step, input)

			// Update execution with current step tracking
			if tracker != nil {
				exec.CurrentStepID = tracker.GetHierarchicalStepID()
				callStack := tracker.GetCallStackCopy()
				if callStackJSON, err := json.Marshal(callStack); err == nil {
					exec.CallStack = callStackJSON
				}
			}

			if err != nil {
				// Step failed
				exec.Status = storage.StatusFailed
				now := time.Now()
				exec.CompletedAt = &now
				e.storage.UpdateExecution(ctx, exec)

				if e.wsHub != nil {
					e.wsHub.Broadcast(websocket.NewWorkflowMessage(
						websocket.MessageTypeWorkflowFailed,
						exec.ID.String(),
						exec.WorkflowID.String(),
						step.Name,
						string(storage.StatusFailed),
						fmt.Sprintf("Step failed: %v", err),
					))
				}
				return
			}

			// Broadcast step completed
			if e.wsHub != nil {
				e.wsHub.Broadcast(websocket.NewWorkflowMessage(
					websocket.MessageTypeWorkflowStep,
					exec.ID.String(),
					exec.WorkflowID.String(),
					step.Name,
					"completed",
					fmt.Sprintf("Step completed: %s", step.Name),
				))
			}
		}
	}

	// All steps completed successfully
	exec.Status = storage.StatusSuccess
	now := time.Now()
	exec.CompletedAt = &now

	if tracker != nil {
		exec.CurrentStepID = tracker.GetHierarchicalStepID()
		callStack := tracker.GetCallStackCopy()
		if callStackJSON, err := json.Marshal(callStack); err == nil {
			exec.CallStack = callStackJSON
		}
	}

	e.storage.UpdateExecution(ctx, exec)

	if e.wsHub != nil {
		e.wsHub.Broadcast(websocket.NewWorkflowMessage(
			websocket.MessageTypeWorkflowCompleted,
			exec.ID.String(),
			exec.WorkflowID.String(),
			"",
			string(storage.StatusSuccess),
			"Workflow execution completed successfully",
		))
	}
}

func (e *Engine) executeStep(ctx context.Context, executionID uuid.UUID, index int, step *definition.Step, input map[string]any) (map[string]any, error) {
	// Get tracker for this execution
	e.runningMu.RLock()
	tracker, exists := e.executionTrackers[executionID]
	e.runningMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("execution tracker not found for execution %s", executionID)
	}

	// Update current step in tracker
	tracker.SetCurrentStep(step.Number)

	stepID := uuid.New()
	inputJSON, _ := json.Marshal(input)

	// Get the hierarchical step ID
	hierarchicalID := tracker.GetHierarchicalStepID()

	stepExec := &storage.ExecutionStep{
		ID:                 stepID,
		ExecutionID:        executionID,
		StepIndex:          index,
		StepName:           step.Name,
		HierarchicalStepID: hierarchicalID,
		Depth:              tracker.GetDepth(),
		Status:             storage.StatusRunning,
		Input:              inputJSON,
		StartedAt:          time.Now(),
	}

	e.storage.CreateExecutionStep(ctx, stepExec)
	e.publishEvent(ctx, executionID, "step.started", map[string]any{
		"step_index":           index,
		"step_name":            step.Name,
		"hierarchical_step_id": hierarchicalID,
		"depth":                tracker.GetDepth(),
	})

	// Execute step
	output, err := e.executor.Execute(ctx, step, input)

	now := time.Now()
	stepExec.CompletedAt = &now

	if err != nil {
		stepExec.Status = storage.StatusFailed
		stepExec.Error = err.Error()
		e.storage.UpdateExecutionStep(ctx, stepExec)
		e.publishEvent(ctx, executionID, "step.failed", map[string]any{
			"step_index":           index,
			"step_name":            step.Name,
			"hierarchical_step_id": hierarchicalID,
			"error":                err.Error(),
		})
		return nil, err
	}

	stepExec.Status = storage.StatusSuccess
	outputJSON, _ := json.Marshal(output)
	stepExec.Output = outputJSON
	e.storage.UpdateExecutionStep(ctx, stepExec)
	e.publishEvent(ctx, executionID, "step.completed", map[string]any{
		"step_index":           index,
		"step_name":            step.Name,
		"hierarchical_step_id": hierarchicalID,
		"output":               output,
	})

	return output, nil
}

func (e *Engine) handleStepError(ctx context.Context, exec *storage.WorkflowExecution, step *definition.Step, err error) {
	now := time.Now()
	exec.Status = storage.StatusFailed
	exec.CompletedAt = &now
	exec.Error = err.Error()
	e.storage.UpdateExecution(ctx, exec)
	e.publishEvent(ctx, exec.ID, "execution.failed", map[string]any{"error": err.Error()})
}

func (e *Engine) publishEvent(ctx context.Context, executionID uuid.UUID, eventType string, payload map[string]any) {
	payloadJSON, _ := json.Marshal(payload)
	event := &storage.ExecutionEvent{
		ID:          uuid.New(),
		ExecutionID: executionID,
		EventType:   eventType,
		Payload:     payloadJSON,
		Timestamp:   time.Now(),
	}
	e.storage.CreateExecutionEvent(ctx, event)
	e.streamer.Broadcast(executionID, event)
}

func (e *Engine) GetExecutionStatus(ctx context.Context, executionID uuid.UUID) (*storage.WorkflowExecution, []storage.ExecutionStep, error) {
	exec, err := e.storage.GetExecution(ctx, executionID)
	if err != nil {
		return nil, nil, err
	}

	steps, err := e.storage.GetExecutionSteps(ctx, executionID)
	if err != nil {
		return nil, nil, err
	}

	return exec, steps, nil
}

func (e *Engine) SetLogger(logger *zap.Logger) {
	e.logger = logger
}
