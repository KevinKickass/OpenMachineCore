package machine

import "time" // Hinzufügen

type State string

const (
	StateStopped   State = "stopped"
	StateHoming    State = "homing"
	StateReady     State = "ready"
	StateRunning   State = "running"
	StateStopping  State = "stopping"
	StateError     State = "error"
	StateEmergency State = "emergency"
)

type Command string

const (
	CommandHome  Command = "home"
	CommandStart Command = "start"
	CommandStop  Command = "stop"
	CommandReset Command = "reset"
)

type MachineStatus struct {
	State            State     `json:"state"`
	CurrentWorkflow  string    `json:"current_workflow,omitempty"`
	ExecutionID      string    `json:"execution_id,omitempty"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	ProductionCycles int       `json:"production_cycles"`
	LastStateChange  time.Time `json:"last_state_change"`
}
