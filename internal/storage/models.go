package storage

import (
	"time"

	"github.com/google/uuid"
)

type DeviceProfile struct {
	ID          uuid.UUID `json:"id"`
	ProfileName string    `json:"profile_name"`
	Vendor      string    `json:"vendor"`
	Model       string    `json:"model"`
	Definition  []byte    `json:"definition"` // JSONB
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Device struct {
	ID         uuid.UUID  `json:"id"`
	DeviceName string     `json:"device_name"`
	ProfileID  *uuid.UUID `json:"profile_id"`
	IPAddress  string     `json:"ip_address"`
	Port       int        `json:"port"`
	UnitID     int        `json:"unit_id"`
	Enabled    bool       `json:"enabled"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type IOMapping struct {
	ID           uuid.UUID `json:"id"`
	DeviceID     uuid.UUID `json:"device_id"`
	LogicalName  string    `json:"logical_name"`
	RegisterName string    `json:"register_name"`
	CreatedAt    time.Time `json:"created_at"`
}

type Workflow struct {
	ID           uuid.UUID `json:"id"`
	WorkflowName string    `json:"workflow_name"`
	Definition   []byte    `json:"definition"` // JSONB
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
