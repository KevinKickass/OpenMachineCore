package definition

import (
	"encoding/json"
	"fmt"
	"time"
)

type Workflow struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	ProgramName string            `json:"program_name"` // "main", "sub_pick", etc.
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version"`
	Steps       []Step            `json:"steps"`
	Variables   map[string]string `json:"variables,omitempty"`
	Loop        *LoopConfig       `json:"loop,omitempty"`
}

type LoopConfig struct {
	Enabled  bool   `json:"enabled"`
	MaxCount int    `json:"max_count,omitempty"`
	OnError  string `json:"on_error,omitempty"`
}

type Step struct {
	Number string   `json:"number"` // "10", "20", "30.1" for parallel branches
	Name   string   `json:"name"`
	Type   StepType `json:"type"`

	// Device Step
	DeviceID   string         `json:"device_id,omitempty"`
	Operation  string         `json:"operation,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`

	// Workflow Step (Sub-Workflow)
	WorkflowID string `json:"workflow_id,omitempty"`

	// Common
	Condition string        `json:"condition,omitempty"`
	OnError   ErrorStrategy `json:"on_error,omitempty"`
	Timeout   Duration      `json:"timeout,omitempty"`
}

// Duration is a wrapper around time.Duration that supports JSON string parsing
type Duration struct {
	time.Duration
}

// UnmarshalJSON parses duration from string like "2s", "100ms", etc.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("invalid duration type: %T", value)
	}
}

// MarshalJSON serializes duration as string
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

type StepType string

const (
	StepTypeDevice   StepType = "device"
	StepTypeWorkflow StepType = "workflow"
	StepTypeWait     StepType = "wait"
)

type ErrorStrategy string

const (
	ErrorStrategyFail     ErrorStrategy = "fail"
	ErrorStrategyRetry    ErrorStrategy = "retry"
	ErrorStrategySkip     ErrorStrategy = "skip"
	ErrorStrategyContinue ErrorStrategy = "continue"
)

func ParseWorkflow(data []byte) (*Workflow, error) {
	var wf Workflow
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, err
	}
	return &wf, nil
}

func (wf *Workflow) ToJSON() ([]byte, error) {
	return json.Marshal(wf)
}

// CallFrame represents a single level in the execution call stack
type CallFrame struct {
	WorkflowID  string `json:"workflow_id"`
	ProgramName string `json:"program_name"`
	StepNumber  string `json:"step_number"`
}

// ExecutionState tracks the current execution state including call stack
type ExecutionState struct {
	CallStack          []CallFrame `json:"call_stack"`
	CurrentStepNumber  string      `json:"current_step_number"`
	HierarchicalStepID string      `json:"hierarchical_step_id"`
	Depth              int         `json:"depth"`
}

// BuildHierarchicalStepID constructs the full hierarchical step ID from call stack
// Example output: "main:S10:sub_pick:S20:sub_gripper:S5"
func BuildHierarchicalStepID(callStack []CallFrame) string {
	if len(callStack) == 0 {
		return ""
	}

	parts := []string{}
	for _, frame := range callStack {
		parts = append(parts, frame.ProgramName)
		parts = append(parts, "S"+frame.StepNumber)
	}

	// Build the hierarchical ID by joining all parts with colons
	result := ""
	for i, part := range parts {
		if i == 0 {
			result = part
		} else {
			result += ":" + part
		}
	}
	return result
}
