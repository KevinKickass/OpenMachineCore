package definition

import (
    "encoding/json"
    "fmt"
    "time"
)

type Workflow struct {
    ID          string              `json:"id"`
    Name        string              `json:"name"`
    Description string              `json:"description,omitempty"`
    Version     string              `json:"version"`
    Steps       []Step              `json:"steps"`
    Variables   map[string]string   `json:"variables,omitempty"`
    Loop        *LoopConfig         `json:"loop,omitempty"`
}

type LoopConfig struct {
    Enabled    bool   `json:"enabled"`
    MaxCount   int    `json:"max_count,omitempty"`
    OnError    string `json:"on_error,omitempty"`
}

type Step struct {
    Name        string            `json:"name"`
    Type        StepType          `json:"type"`
    
    // Device Step
    DeviceID    string            `json:"device_id,omitempty"`
    Operation   string            `json:"operation,omitempty"`
    Parameters  map[string]any    `json:"parameters,omitempty"`
    
    // Workflow Step (Sub-Workflow)
    WorkflowID  string            `json:"workflow_id,omitempty"`
    
    // Common
    Condition   string            `json:"condition,omitempty"`
    OnError     ErrorStrategy     `json:"on_error,omitempty"`
    Timeout     Duration          `json:"timeout,omitempty"`
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
    ErrorStrategyFail    ErrorStrategy = "fail"
    ErrorStrategyRetry   ErrorStrategy = "retry"
    ErrorStrategySkip    ErrorStrategy = "skip"
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
