package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/storage"
	"github.com/KevinKickass/OpenMachineCore/internal/workflow/definition"
	"github.com/KevinKickass/OpenMachineCore/internal/workflow/executor"
	"github.com/KevinKickass/OpenMachineCore/internal/workflow/streaming"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Engine struct {
	storage  *storage.PostgresClient
	executor *executor.StepExecutor
	streamer *streaming.EventStreamer
	logger   *zap.Logger

	runningMu       sync.RWMutex
	runningContexts map[uuid.UUID]context.CancelFunc
}

func NewEngine(storage *storage.PostgresClient, executor *executor.StepExecutor, streamer *streaming.EventStreamer, logger *zap.Logger) *Engine {
	return &Engine{
		storage:         storage,
		executor:        executor,
		streamer:        streamer,
		runningContexts: make(map[uuid.UUID]context.CancelFunc),
		logger:          logger,
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

	// Create cancellable context for this execution
	execCtx, cancel := context.WithCancel(context.Background())

	e.runningMu.Lock()
	e.runningContexts[executionID] = cancel
	e.runningMu.Unlock()

	// Execute asynchronously
	go func() {
		defer func() {
			e.runningMu.Lock()
			delete(e.runningContexts, executionID)
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

func (e *Engine) runExecution(ctx context.Context, exec *storage.WorkflowExecution, workflow *definition.Workflow, input map[string]any) {
	exec.Status = storage.StatusRunning
	e.storage.UpdateExecution(ctx, exec)
	e.publishEvent(ctx, exec.ID, "execution.started", nil)

	stepInput := input
	for i, step := range workflow.Steps {
		exec.CurrentStep = i
		e.storage.UpdateExecution(ctx, exec)

		stepResult, err := e.executeStep(ctx, exec.ID, i, &step, stepInput)
		if err != nil {
			e.handleStepError(ctx, exec, &step, err)
			return
		}

		stepInput = stepResult
	}

	// Workflow completed successfully
	now := time.Now()
	exec.Status = storage.StatusSuccess
	exec.CompletedAt = &now
	outputJSON, _ := json.Marshal(stepInput)
	exec.Output = outputJSON
	e.storage.UpdateExecution(ctx, exec)
	e.publishEvent(ctx, exec.ID, "execution.completed", stepInput)
}

func (e *Engine) executeStep(ctx context.Context, executionID uuid.UUID, index int, step *definition.Step, input map[string]any) (map[string]any, error) {
	stepID := uuid.New()
	inputJSON, _ := json.Marshal(input)

	stepExec := &storage.ExecutionStep{
		ID:          stepID,
		ExecutionID: executionID,
		StepIndex:   index,
		StepName:    step.Name,
		Status:      storage.StatusRunning,
		Input:       inputJSON,
		StartedAt:   time.Now(),
	}

	e.storage.CreateExecutionStep(ctx, stepExec)
	e.publishEvent(ctx, executionID, "step.started", map[string]any{
		"step_index": index,
		"step_name":  step.Name,
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
			"step_index": index,
			"step_name":  step.Name,
			"error":      err.Error(),
		})
		return nil, err
	}

	stepExec.Status = storage.StatusSuccess
	outputJSON, _ := json.Marshal(output)
	stepExec.Output = outputJSON
	e.storage.UpdateExecutionStep(ctx, stepExec)
	e.publishEvent(ctx, executionID, "step.completed", map[string]any{
		"step_index": index,
		"step_name":  step.Name,
		"output":     output,
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
