package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KevinKickass/OpenMachineCore/internal/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SaveDeviceComposition saves a device composition to database
func (p *PostgresClient) SaveDeviceComposition(ctx context.Context, comp types.DeviceComposition) (uuid.UUID, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	compJSON, err := json.Marshal(comp.Composition)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to marshal composition: %w", err)
	}

	ioMappingJSON, err := json.Marshal(comp.IOMapping)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to marshal io_mapping: %w", err)
	}

	// Insert into devices table
	var deviceID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO devices (device_name, ip_address, port, unit_id, enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, comp.InstanceID,
		comp.Composition.Coupler.IPAddress,
		comp.Composition.Coupler.Port,
		comp.Composition.Coupler.UnitID,
		true,
	).Scan(&deviceID)

	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert device: %w", err)
	}

	// Insert into device_compositions table
	_, err = tx.Exec(ctx, `
		INSERT INTO device_compositions (device_id, instance_id, composition, io_mapping)
		VALUES ($1, $2, $3, $4)
	`, deviceID, comp.InstanceID, compJSON, ioMappingJSON)

	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to save composition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return deviceID, nil
}

// LoadAllDeviceCompositions loads all enabled device compositions
func (p *PostgresClient) LoadAllDeviceCompositions(ctx context.Context) ([]types.DeviceComposition, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT 
			dc.instance_id,
			dc.composition,
			dc.io_mapping
		FROM devices d
		JOIN device_compositions dc ON d.id = dc.device_id
		WHERE d.enabled = true
	`)

	if err != nil {
		return nil, fmt.Errorf("failed to query devices: %w", err)
	}
	defer rows.Close()

	compositions := make([]types.DeviceComposition, 0)

	for rows.Next() {
		var comp types.DeviceComposition
		var compJSON, ioMappingJSON []byte

		err := rows.Scan(&comp.InstanceID, &compJSON, &ioMappingJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan device: %w", err)
		}

		if err := json.Unmarshal(compJSON, &comp.Composition); err != nil {
			return nil, fmt.Errorf("failed to unmarshal composition: %w", err)
		}

		if err := json.Unmarshal(ioMappingJSON, &comp.IOMapping); err != nil {
			return nil, fmt.Errorf("failed to unmarshal io_mapping: %w", err)
		}

		compositions = append(compositions, comp)
	}

	return compositions, nil
}

// DeleteDevice removes a device from database
func (p *PostgresClient) DeleteDevice(ctx context.Context, instanceID string) error {
	result, err := p.pool.Exec(ctx, `
		DELETE FROM devices 
		WHERE device_name = $1
	`, instanceID)

	if err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// SaveOrUpdateDeviceComposition saves or updates a device composition
func (p *PostgresClient) SaveOrUpdateDeviceComposition(ctx context.Context, comp types.DeviceComposition) (uuid.UUID, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	compJSON, err := json.Marshal(comp.Composition)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to marshal composition: %w", err)
	}

	ioMappingJSON, err := json.Marshal(comp.IOMapping)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to marshal io_mapping: %w", err)
	}

	// Upsert into devices table
	var deviceID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO devices (device_name, ip_address, port, unit_id, enabled)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (device_name) 
		DO UPDATE SET 
			ip_address = EXCLUDED.ip_address,
			port = EXCLUDED.port,
			unit_id = EXCLUDED.unit_id,
			enabled = EXCLUDED.enabled,
			updated_at = NOW()
		RETURNING id
	`, comp.InstanceID,
		comp.Composition.Coupler.IPAddress,
		comp.Composition.Coupler.Port,
		comp.Composition.Coupler.UnitID,
		true,
	).Scan(&deviceID)

	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to upsert device: %w", err)
	}

	// Upsert into device_compositions table
	_, err = tx.Exec(ctx, `
		INSERT INTO device_compositions (device_id, instance_id, composition, io_mapping)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (instance_id)
		DO UPDATE SET
			composition = EXCLUDED.composition,
			io_mapping = EXCLUDED.io_mapping,
			updated_at = NOW()
	`, deviceID, comp.InstanceID, compJSON, ioMappingJSON)

	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to upsert composition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return deviceID, nil
}
