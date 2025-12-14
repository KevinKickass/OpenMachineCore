package websocket

import "time"

// MessageType defines the type of WebSocket message
type MessageType string

const (
	// Device-related messages
	MessageTypeDeviceIO        MessageType = "device_io"
	MessageTypeDeviceConnected MessageType = "device_connected"
	MessageTypeDeviceError     MessageType = "device_error"

	// Machine state messages
	MessageTypeMachineState MessageType = "machine_state"

	// Workflow execution messages
	MessageTypeWorkflowStarted   MessageType = "workflow_started"
	MessageTypeWorkflowStep      MessageType = "workflow_step"
	MessageTypeWorkflowCompleted MessageType = "workflow_completed"
	MessageTypeWorkflowFailed    MessageType = "workflow_failed"
	MessageTypeWorkflowCancelled MessageType = "workflow_cancelled"

	// System messages
	MessageTypeSystemStatus MessageType = "system_status"
)

// Message represents a WebSocket message
type Message struct {
	Type      MessageType `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// DeviceIOData represents device I/O update data
type DeviceIOData struct {
	DeviceID string                 `json:"device_id"`
	Address  string                 `json:"address"`
	Value    interface{}            `json:"value"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// MachineStateData represents machine state change data
type MachineStateData struct {
	State    string `json:"state"`
	Previous string `json:"previous_state"`
}

// WorkflowExecutionData represents workflow execution event data
type WorkflowExecutionData struct {
	ExecutionID string                 `json:"execution_id"`
	WorkflowID  string                 `json:"workflow_id"`
	StepName    string                 `json:"step_name,omitempty"`
	Status      string                 `json:"status"`
	Message     string                 `json:"message,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// NewMessage creates a new message with current timestamp
func NewMessage(msgType MessageType, data interface{}) Message {
	return Message{
		Type:      msgType,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// Helper functions for creating specific message types

func NewDeviceIOMessage(deviceID, address string, value interface{}) Message {
	return NewMessage(MessageTypeDeviceIO, DeviceIOData{
		DeviceID: deviceID,
		Address:  address,
		Value:    value,
	})
}

func NewMachineStateMessage(newState, previousState string) Message {
	return NewMessage(MessageTypeMachineState, MachineStateData{
		State:    newState,
		Previous: previousState,
	})
}

func NewWorkflowMessage(msgType MessageType, executionID, workflowID, stepName, status, message string) Message {
	return NewMessage(msgType, WorkflowExecutionData{
		ExecutionID: executionID,
		WorkflowID:  workflowID,
		StepName:    stepName,
		Status:      status,
		Message:     message,
	})
}
