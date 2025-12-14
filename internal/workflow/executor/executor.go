package executor

import (
    "context"
    "fmt"
    "time"

    "github.com/KevinKickass/OpenMachineCore/internal/devices"
    "github.com/KevinKickass/OpenMachineCore/internal/modbus"
    "github.com/KevinKickass/OpenMachineCore/internal/storage"
    "github.com/KevinKickass/OpenMachineCore/internal/workflow/definition"
    "github.com/google/uuid"
)

type StepExecutor struct {
    deviceManager *devices.Manager
    storage       *storage.PostgresClient  // NEU für Sub-Workflow Laden
}

func NewStepExecutor(dm *devices.Manager, storage *storage.PostgresClient) *StepExecutor {
    return &StepExecutor{
        deviceManager: dm,
        storage:       storage,
    }
}

func (e *StepExecutor) Execute(ctx context.Context, step *definition.Step, input map[string]any) (map[string]any, error) {
    switch step.Type {
    case definition.StepTypeDevice:
        return e.executeDeviceStep(ctx, step, input)
    case definition.StepTypeWorkflow:
        return e.executeWorkflowStep(ctx, step, input)  // NEU
    case definition.StepTypeWait:
        return e.executeWaitStep(ctx, step, input)
    default:
        return nil, fmt.Errorf("unsupported step type: %s", step.Type)
    }
}

func (e *StepExecutor) executeDeviceStep(ctx context.Context, step *definition.Step, input map[string]any) (map[string]any, error) {
    if step.Timeout.Duration > 0 { 
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, step.Timeout.Duration)
        defer cancel()
    }

    // Get device by name (instance_id)
    device, exists := e.deviceManager.GetDeviceByName(step.DeviceID)
    if !exists {
        return nil, fmt.Errorf("device not found: %s", step.DeviceID)
    }

    // Merge step parameters with input
    params := make(map[string]any)
    for k, v := range step.Parameters {
        params[k] = v
    }
    for k, v := range input {
        params[k] = v
    }

    // Execute operation based on type
    result, err := e.executeOperation(ctx, device, step.Operation, params)
    if err != nil {
        return nil, fmt.Errorf("device operation failed: %w", err)
    }

    return result, nil
}

func (e *StepExecutor) executeOperation(ctx context.Context, device *modbus.Device, operation string, params map[string]any) (map[string]any, error) {
    switch operation {
    case "read":
        return e.executeRead(ctx, device, params)
    case "write":
        return e.executeWrite(ctx, device, params)
    case "read_logical":
        return e.executeReadLogical(ctx, device, params)
    case "write_logical":
        return e.executeWriteLogical(ctx, device, params)
    case "read_register":
        return e.executeReadRegister(ctx, device, params)
    case "write_register":
        return e.executeWriteRegister(ctx, device, params)
    default:
        return nil, fmt.Errorf("unsupported operation: %s", operation)
    }
}

func (e *StepExecutor) executeRead(ctx context.Context, device *modbus.Device, params map[string]any) (map[string]any, error) {
    registerType, ok := params["register_type"].(string)
    if !ok {
        return nil, fmt.Errorf("missing or invalid register_type parameter")
    }

    address, ok := params["address"].(float64)
    if !ok {
        return nil, fmt.Errorf("missing or invalid address parameter")
    }

    count := uint16(1)
    if c, ok := params["count"].(float64); ok {
        count = uint16(c)
    }

    unitID := uint8(device.Profile.Connection.UnitID)

    var values interface{}
    var err error

    switch registerType {
    case "holding":
        values, err = device.Client.ReadHoldingRegisters(ctx, unitID, uint16(address), count)
    case "input":
        values, err = device.Client.ReadInputRegisters(ctx, unitID, uint16(address), count)
    default:
        return nil, fmt.Errorf("invalid register_type: %s (only 'holding' and 'input' supported)", registerType)
    }

    if err != nil {
        return nil, err
    }

    return map[string]any{
        "values": values,
    }, nil
}

func (e *StepExecutor) executeWrite(ctx context.Context, device *modbus.Device, params map[string]any) (map[string]any, error) {
    registerType, ok := params["register_type"].(string)
    if !ok {
        return nil, fmt.Errorf("missing or invalid register_type parameter")
    }

    address, ok := params["address"].(float64)
    if !ok {
        return nil, fmt.Errorf("missing or invalid address parameter")
    }

    value, ok := params["value"].(float64)
    if !ok {
        return nil, fmt.Errorf("missing or invalid value parameter")
    }

    unitID := uint8(device.Profile.Connection.UnitID)

    if registerType != "holding" {
        return nil, fmt.Errorf("invalid register_type for write: %s (only 'holding' supported)", registerType)
    }

    err := device.Client.WriteSingleRegister(ctx, unitID, uint16(address), uint16(value))
    if err != nil {
        return nil, err
    }

    return map[string]any{
        "success": true,
        "address": uint16(address),
        "value":   uint16(value),
    }, nil
}

func (e *StepExecutor) executeReadRegister(ctx context.Context, device *modbus.Device, params map[string]any) (map[string]any, error) {
    register, ok := params["register"].(string)
    if !ok {
        return nil, fmt.Errorf("missing or invalid register parameter")
    }

    value, err := device.ReadRegister(ctx, register)
    if err != nil {
        return nil, err
    }

    return map[string]any{
        "register": register,
        "value":    value,
    }, nil
}

func (e *StepExecutor) executeWriteRegister(ctx context.Context, device *modbus.Device, params map[string]any) (map[string]any, error) {
    register, ok := params["register"].(string)
    if !ok {
        return nil, fmt.Errorf("missing or invalid register parameter")
    }

    value, ok := params["value"]
    if !ok {
        return nil, fmt.Errorf("missing value parameter")
    }

    if err := device.WriteRegister(ctx, register, value); err != nil {
        return nil, err
    }

    return map[string]any{
        "register": register,
        "value":    value,
        "success":  true,
    }, nil
}

func (e *StepExecutor) executeReadLogical(ctx context.Context, device *modbus.Device, params map[string]any) (map[string]any, error) {
    register, ok := params["register"].(string)
    if !ok {
        return nil, fmt.Errorf("missing or invalid register parameter")
    }

    value, err := device.ReadLogical(ctx, register)
    if err != nil {
        return nil, err
    }

    return map[string]any{
        "register": register,
        "value":    value,
    }, nil
}

func (e *StepExecutor) executeWriteLogical(ctx context.Context, device *modbus.Device, params map[string]any) (map[string]any, error) {
    register, ok := params["register"].(string)
    if !ok {
        return nil, fmt.Errorf("missing or invalid register parameter")
    }

    value, ok := params["value"]
    if !ok {
        return nil, fmt.Errorf("missing value parameter")
    }

    if err := device.WriteLogical(ctx, register, value); err != nil {
        return nil, err
    }

    return map[string]any{
        "register": register,
        "value":    value,
        "success":  true,
    }, nil
}

func (e *StepExecutor) executeWaitStep(ctx context.Context, step *definition.Step, input map[string]any) (map[string]any, error) {
    duration := step.Timeout.Duration  // Zugriff auf .Duration
    if duration == 0 {
        duration = 1 * time.Second
    }

    select {
    case <-time.After(duration):
        return input, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

func (e *StepExecutor) executeWorkflowStep(ctx context.Context, step *definition.Step, input map[string]any) (map[string]any, error) {
    if step.Timeout.Duration > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, step.Timeout.Duration)
        defer cancel()
    }

    // Parse workflow ID
    workflowID, err := uuid.Parse(step.WorkflowID)
    if err != nil {
        return nil, fmt.Errorf("invalid workflow_id: %w", err)
    }

    // Load sub-workflow
    workflow, _, err := e.storage.LoadWorkflow(ctx, workflowID)
    if err != nil {
        return nil, fmt.Errorf("failed to load sub-workflow: %w", err)
    }

    // Parse workflow definition
    subWorkflow, err := definition.ParseWorkflow(workflow.Definition)
    if err != nil {
        return nil, fmt.Errorf("failed to parse sub-workflow: %w", err)
    }

    // Execute all steps of sub-workflow
    stepInput := input
    for i, subStep := range subWorkflow.Steps {
        result, err := e.Execute(ctx, &subStep, stepInput)
        if err != nil {
            return nil, fmt.Errorf("sub-workflow step %d (%s) failed: %w", i, subStep.Name, err)
        }
        stepInput = result
    }

    return stepInput, nil
}