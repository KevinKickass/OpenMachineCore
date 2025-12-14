package types

type DeviceComposition struct {
	InstanceID string              `json:"instance_id"`
	Composition CompositionConfig  `json:"composition"`
	IOMapping  map[string]string   `json:"io_mapping"`
}

type CompositionConfig struct {
	Coupler   CouplerConfig    `json:"coupler"`
	Terminals []TerminalConfig `json:"terminals"`
}

type CouplerConfig struct {
	Module    string `json:"module"`
	IPAddress string `json:"ip_address"`
	Port      int    `json:"port"`
	UnitID    int    `json:"unit_id"`
}

type TerminalConfig struct {
	Position int    `json:"position"`
	Module   string `json:"module"`
	Prefix   string `json:"prefix"`
}

type ModuleDefinition struct {
	Module        ModuleInfo       `json:"module"`
	ProcessImage  ProcessImageInfo `json:"process_image"`
	Channels      []ChannelInfo    `json:"channels"`
	Registers     []RegisterDefinition `json:"registers,omitempty"`
}

type ModuleInfo struct {
	ID          string `json:"id"`
	Vendor      string `json:"vendor"`
	Model       string `json:"model"`
	Type        string `json:"type"` // coupler, input, output, analog
	Version     string `json:"version"`
	Description string `json:"description"`
}

type ProcessImageInfo struct {
	InputBytes  int `json:"input_bytes"`
	OutputBytes int `json:"output_bytes"`
}

type ChannelInfo struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"` // digital_input, digital_output, analog_input, etc.
	BitOffset   int    `json:"bit_offset"`
	Description string `json:"description"`
}
