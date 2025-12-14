package system

import "fmt"

type SystemState int

const (
	StateInitializing SystemState = iota
	StateRunning
	StateUpdating
	StateStopping
	StateStopped
	StateError
)

func (s SystemState) String() string {
	switch s {
	case StateInitializing:
		return "INITIALIZING"
	case StateRunning:
		return "RUNNING"
	case StateUpdating:
		return "UPDATING"
	case StateStopping:
		return "STOPPING"
	case StateStopped:
		return "STOPPED"
	case StateError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

type UpdateProgress struct {
	Phase     string  `json:"phase"`
	Progress  int     `json:"progress"` // 0-100
	Message   string  `json:"message"`
	StartedAt int64   `json:"started_at"`
}

type SystemStatus struct {
	State          SystemState    `json:"state"`
	UpdateProgress UpdateProgress `json:"update_progress,omitempty"`
	Timestamp      int64          `json:"timestamp"`
	Error          string         `json:"error,omitempty"`
}

func (s *SystemStatus) ToProto() interface{} {
	// Wird später für gRPC verwendet
	return s
}

type StateTransition struct {
	From    SystemState
	To      SystemState
	Allowed bool
	Reason  string
}

func ValidateTransition(from, to SystemState) error {
	validTransitions := map[SystemState][]SystemState{
		StateInitializing: {StateRunning, StateError},
		StateRunning:      {StateUpdating, StateStopping, StateError},
		StateUpdating:     {StateRunning, StateError},
		StateStopping:     {StateStopped, StateError},
		StateStopped:      {StateInitializing},
		StateError:        {StateInitializing, StateStopped},
	}

	allowed, exists := validTransitions[from]
	if !exists {
		return fmt.Errorf("invalid current state: %s", from)
	}

	for _, validTo := range allowed {
		if validTo == to {
			return nil
		}
	}

	return fmt.Errorf("invalid state transition: %s -> %s", from, to)
}
