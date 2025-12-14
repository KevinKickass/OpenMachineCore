package interfaces

import (
    "context"

    "github.com/KevinKickass/OpenMachineCore/internal/config"
    "github.com/KevinKickass/OpenMachineCore/internal/devices"
    "github.com/KevinKickass/OpenMachineCore/internal/machine"
    "github.com/KevinKickass/OpenMachineCore/internal/storage"
    "github.com/KevinKickass/OpenMachineCore/internal/workflow/engine"
)

// SystemStatus represents the current system state
type SystemStatus struct {
    State            string `json:"state"`
    ActiveWorkflow   string `json:"active_workflow,omitempty"`
    DeviceCount      int    `json:"device_count"`
    ConnectedDevices int    `json:"connected_devices"`
}

type LifecycleManager interface {
    Config() *config.Config
    Storage() *storage.PostgresClient
    DeviceManager() *devices.Manager
    WorkflowEngine() *engine.Engine
    MachineController() *machine.Controller
    GetCurrentStatus() SystemStatus
    TriggerUpdate(workflowPath string) error
    Shutdown(ctx context.Context) error
}
