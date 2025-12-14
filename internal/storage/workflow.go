package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KevinKickass/OpenMachineCore/internal/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

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
