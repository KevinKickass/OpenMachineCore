package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Workflow execution types
type WorkflowExecution struct {
	ID          uuid.UUID
	WorkflowID  uuid.UUID
	Status      ExecutionStatus
	CurrentStep int
	Input       json.RawMessage
	Output      json.RawMessage
	Error       string
	StartedAt   time.Time
	CompletedAt *time.Time
}

type ExecutionStatus string

const (
	StatusPending   ExecutionStatus = "pending"
	StatusRunning   ExecutionStatus = "running"
	StatusSuccess   ExecutionStatus = "success"
	StatusFailed    ExecutionStatus = "failed"
	StatusCancelled ExecutionStatus = "cancelled"
)

type ExecutionStep struct {
	ID          uuid.UUID
	ExecutionID uuid.UUID
	StepIndex   int
	StepName    string
	Status      ExecutionStatus
	Input       json.RawMessage
	Output      json.RawMessage
	Error       string
	StartedAt   time.Time
	CompletedAt *time.Time
}

type ExecutionEvent struct {
	ID          uuid.UUID
	ExecutionID uuid.UUID
	EventType   string
	Payload     json.RawMessage
	Timestamp   time.Time
}

// SaveWorkflow stores a workflow with its compositions
func (p *PostgresClient) SaveWorkflow(ctx context.Context, workflow *Workflow, compositions []types.DeviceComposition) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert workflow
	err = tx.QueryRow(ctx, `
        INSERT INTO workflows (workflow_name, definition, active)
        VALUES ($1, $2, $3)
        RETURNING id
    `, workflow.WorkflowName, workflow.Definition, workflow.Active).Scan(&workflow.ID)

	if err != nil {
		return fmt.Errorf("failed to insert workflow: %w", err)
	}

	// Insert compositions
	for _, comp := range compositions {
		compJSON, err := json.Marshal(comp.Composition)
		if err != nil {
			return fmt.Errorf("failed to marshal composition: %w", err)
		}

		ioMappingJSON, err := json.Marshal(comp.IOMapping)
		if err != nil {
			return fmt.Errorf("failed to marshal io_mapping: %w", err)
		}

		_, err = tx.Exec(ctx, `
            INSERT INTO workflow_compositions (workflow_id, instance_id, composition, io_mapping)
            VALUES ($1, $2, $3, $4)
        `, workflow.ID, comp.InstanceID, compJSON, ioMappingJSON)

		if err != nil {
			return fmt.Errorf("failed to insert composition: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// LoadWorkflow loads workflow with compositions
func (p *PostgresClient) LoadWorkflow(ctx context.Context, workflowID uuid.UUID) (*Workflow, []types.DeviceComposition, error) {
	// Load workflow
	var workflow Workflow
	err := p.pool.QueryRow(ctx, `
        SELECT id, workflow_name, definition, active, created_at, updated_at
        FROM workflows
        WHERE id = $1
    `, workflowID).Scan(
		&workflow.ID,
		&workflow.WorkflowName,
		&workflow.Definition,
		&workflow.Active,
		&workflow.CreatedAt,
		&workflow.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, fmt.Errorf("workflow not found: %s", workflowID)
		}
		return nil, nil, fmt.Errorf("failed to load workflow: %w", err)
	}

	// Load compositions
	rows, err := p.pool.Query(ctx, `
        SELECT instance_id, composition, io_mapping
        FROM workflow_compositions
        WHERE workflow_id = $1
        ORDER BY created_at
    `, workflowID)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to load compositions: %w", err)
	}
	defer rows.Close()

	compositions := make([]types.DeviceComposition, 0)
	for rows.Next() {
		var comp types.DeviceComposition
		var compJSON, ioMappingJSON []byte

		err := rows.Scan(&comp.InstanceID, &compJSON, &ioMappingJSON)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan composition: %w", err)
		}

		if err := json.Unmarshal(compJSON, &comp.Composition); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal composition: %w", err)
		}

		if err := json.Unmarshal(ioMappingJSON, &comp.IOMapping); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal io_mapping: %w", err)
		}

		compositions = append(compositions, comp)
	}

	return &workflow, compositions, nil
}

// GetActiveWorkflow returns the currently active workflow
func (p *PostgresClient) GetActiveWorkflow(ctx context.Context) (*Workflow, []types.DeviceComposition, error) {
	var workflowID uuid.UUID
	err := p.pool.QueryRow(ctx, `
        SELECT id FROM workflows WHERE active = true LIMIT 1
    `).Scan(&workflowID)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, fmt.Errorf("no active workflow")
		}
		return nil, nil, fmt.Errorf("failed to find active workflow: %w", err)
	}

	return p.LoadWorkflow(ctx, workflowID)
}

// ListWorkflows returns all workflows
func (p *PostgresClient) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	rows, err := p.pool.Query(ctx, `
        SELECT id, workflow_name, definition, active, created_at, updated_at
        FROM workflows
        ORDER BY created_at DESC
    `)
	if err != nil {
		return nil, fmt.Errorf("failed to query workflows: %w", err)
	}
	defer rows.Close()

	workflows := make([]Workflow, 0)
	for rows.Next() {
		var wf Workflow
		err := rows.Scan(&wf.ID, &wf.WorkflowName, &wf.Definition, &wf.Active, &wf.CreatedAt, &wf.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan workflow: %w", err)
		}
		workflows = append(workflows, wf)
	}

	return workflows, nil
}

// UpdateWorkflow updates an existing workflow
func (p *PostgresClient) UpdateWorkflow(ctx context.Context, workflow *Workflow) error {
	_, err := p.pool.Exec(ctx, `
        UPDATE workflows
        SET workflow_name = $1, definition = $2, active = $3, updated_at = NOW()
        WHERE id = $4
    `, workflow.WorkflowName, workflow.Definition, workflow.Active, workflow.ID)

	if err != nil {
		return fmt.Errorf("failed to update workflow: %w", err)
	}

	return nil
}

// DeleteWorkflow deletes a workflow and its compositions
func (p *PostgresClient) DeleteWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	_, err := p.pool.Exec(ctx, `
        DELETE FROM workflows WHERE id = $1
    `, workflowID)

	if err != nil {
		return fmt.Errorf("failed to delete workflow: %w", err)
	}

	return nil
}

// ActivateWorkflow activates a workflow and deactivates all others
func (p *PostgresClient) ActivateWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Deactivate all workflows
	_, err = tx.Exec(ctx, `UPDATE workflows SET active = false`)
	if err != nil {
		return fmt.Errorf("failed to deactivate workflows: %w", err)
	}

	// Activate target workflow
	_, err = tx.Exec(ctx, `UPDATE workflows SET active = true WHERE id = $1`, workflowID)
	if err != nil {
		return fmt.Errorf("failed to activate workflow: %w", err)
	}

	return tx.Commit(ctx)
}

// CreateExecution creates a new workflow execution record
func (p *PostgresClient) CreateExecution(ctx context.Context, exec *WorkflowExecution) error {
	_, err := p.pool.Exec(ctx, `
        INSERT INTO workflow_executions 
        (id, workflow_id, status, current_step, input, started_at)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, exec.ID, exec.WorkflowID, exec.Status, exec.CurrentStep, exec.Input, exec.StartedAt)
	return err
}

// UpdateExecution updates an existing workflow execution
func (p *PostgresClient) UpdateExecution(ctx context.Context, exec *WorkflowExecution) error {
	_, err := p.pool.Exec(ctx, `
        UPDATE workflow_executions
        SET status = $1, current_step = $2, output = $3, error = $4, completed_at = $5
        WHERE id = $6
    `, exec.Status, exec.CurrentStep, exec.Output, exec.Error, exec.CompletedAt, exec.ID)
	return err
}

// GetExecution retrieves a workflow execution by ID
func (p *PostgresClient) GetExecution(ctx context.Context, id uuid.UUID) (*WorkflowExecution, error) {
	var exec WorkflowExecution
	err := p.pool.QueryRow(ctx, `
        SELECT id, workflow_id, status, current_step, input, output, error, started_at, completed_at
        FROM workflow_executions WHERE id = $1
    `, id).Scan(&exec.ID, &exec.WorkflowID, &exec.Status, &exec.CurrentStep,
		&exec.Input, &exec.Output, &exec.Error, &exec.StartedAt, &exec.CompletedAt)

	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("execution not found: %s", id)
	}
	return &exec, err
}

// CreateExecutionStep creates a step execution record
func (p *PostgresClient) CreateExecutionStep(ctx context.Context, step *ExecutionStep) error {
	_, err := p.pool.Exec(ctx, `
        INSERT INTO execution_steps
        (id, execution_id, step_index, step_name, status, input, started_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `, step.ID, step.ExecutionID, step.StepIndex, step.StepName, step.Status, step.Input, step.StartedAt)
	return err
}

// UpdateExecutionStep updates a step execution record
func (p *PostgresClient) UpdateExecutionStep(ctx context.Context, step *ExecutionStep) error {
	_, err := p.pool.Exec(ctx, `
        UPDATE execution_steps
        SET status = $1, output = $2, error = $3, completed_at = $4
        WHERE id = $5
    `, step.Status, step.Output, step.Error, step.CompletedAt, step.ID)
	return err
}

// CreateExecutionEvent creates an execution event for streaming
func (p *PostgresClient) CreateExecutionEvent(ctx context.Context, event *ExecutionEvent) error {
	_, err := p.pool.Exec(ctx, `
        INSERT INTO execution_events (id, execution_id, event_type, payload, timestamp)
        VALUES ($1, $2, $3, $4, $5)
    `, event.ID, event.ExecutionID, event.EventType, event.Payload, event.Timestamp)
	return err
}

// GetExecutionSteps retrieves all steps for an execution
func (p *PostgresClient) GetExecutionSteps(ctx context.Context, executionID uuid.UUID) ([]ExecutionStep, error) {
	rows, err := p.pool.Query(ctx, `
        SELECT id, execution_id, step_index, step_name, status, input, output, error, started_at, completed_at
        FROM execution_steps
        WHERE execution_id = $1
        ORDER BY step_index
    `, executionID)

	if err != nil {
		return nil, fmt.Errorf("failed to query steps: %w", err)
	}
	defer rows.Close()

	steps := make([]ExecutionStep, 0)
	for rows.Next() {
		var step ExecutionStep
		err := rows.Scan(&step.ID, &step.ExecutionID, &step.StepIndex, &step.StepName,
			&step.Status, &step.Input, &step.Output, &step.Error, &step.StartedAt, &step.CompletedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan step: %w", err)
		}
		steps = append(steps, step)
	}

	return steps, nil
}
