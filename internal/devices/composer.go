package devices

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/KevinKickass/OpenMachineCore/internal/types"
	"go.uber.org/zap"
)

type Composer struct {
	searchPaths []string
	logger      *zap.Logger
}

func NewComposer(searchPaths []string, logger *zap.Logger) *Composer {
	return &Composer{
		searchPaths: searchPaths,
		logger:      logger,
	}
}

// ComposeDevice builds a complete device profile from composition
func (c *Composer) ComposeDevice(comp types.DeviceComposition) (*types.DeviceProfileDefinition, error) {
	c.logger.Info("Composing device",
		zap.String("instance_id", comp.InstanceID),
		zap.String("coupler", comp.Composition.Coupler.Module))

	// Load coupler module
	couplerModule, err := c.loadModule(comp.Composition.Coupler.Module)
	if err != nil {
		return nil, fmt.Errorf("failed to load coupler: %w", err)
	}

	if couplerModule.Module.Type != "coupler" {
		return nil, fmt.Errorf("module %s is not a coupler (type: %s)",
			couplerModule.Module.ID, couplerModule.Module.Type)
	}

	// Initialize device profile
	profile := &types.DeviceProfileDefinition{
		DeviceProfile: types.DeviceProfileInfo{
			ID:          comp.InstanceID,
			Vendor:      couplerModule.Module.Vendor,
			Model:       fmt.Sprintf("%s + %d terminals", couplerModule.Module.Model, len(comp.Composition.Terminals)),
			Version:     "1.0",
			Description: fmt.Sprintf("Composed device: %s", comp.InstanceID),
		},
		Connection: types.ConnectionConfig{
			Protocol:       "modbus_tcp",
			Port:           comp.Composition.Coupler.Port,
			UnitID:         comp.Composition.Coupler.UnitID,
			PollIntervalMs: 50,
			TimeoutMs:      1000,
		},
		Registers: make([]types.RegisterDefinition, 0),
		Groups:    make([]types.RegisterGroup, 0),
	}

	// Add coupler registers (diagnostics, status, etc.)
	if len(couplerModule.Registers) > 0 {
		profile.Registers = append(profile.Registers, couplerModule.Registers...)
	}

	// Calculate process image offsets
	inputByteOffset := 0
	outputByteOffset := 0

	// Process each terminal in order
	for i, terminal := range comp.Composition.Terminals {
		c.logger.Debug("Processing terminal",
			zap.Int("position", terminal.Position),
			zap.String("module", terminal.Module),
			zap.String("prefix", terminal.Prefix))

		terminalModule, err := c.loadModule(terminal.Module)
		if err != nil {
			return nil, fmt.Errorf("failed to load terminal at position %d: %w", i, err)
		}

		// Convert channels to registers
		terminalRegisters := c.channelsToRegisters(
			terminalModule,
			terminal.Prefix,
			inputByteOffset,
			outputByteOffset,
		)

		profile.Registers = append(profile.Registers, terminalRegisters...)

		// Update offsets for next terminal
		inputByteOffset += terminalModule.ProcessImage.InputBytes
		outputByteOffset += terminalModule.ProcessImage.OutputBytes
	}

	// Create register groups for efficient polling
	profile.Groups = c.createRegisterGroups(profile.Registers)

	c.logger.Info("Device composition complete",
		zap.String("instance_id", comp.InstanceID),
		zap.Int("total_registers", len(profile.Registers)),
		zap.Int("register_groups", len(profile.Groups)))

	return profile, nil
}

func (c *Composer) loadModule(modulePath string) (*types.ModuleDefinition, error) {
	var data []byte
	var err error
	var foundPath string

	// Search in all configured paths
	for _, searchPath := range c.searchPaths {
		fullPath := filepath.Join(searchPath, modulePath+".json")
		data, err = os.ReadFile(fullPath)
		if err == nil {
			foundPath = fullPath
			break
		}
	}

	if data == nil {
		return nil, fmt.Errorf("module not found: %s (searched in: %v)", modulePath, c.searchPaths)
	}

	var module types.ModuleDefinition
	if err := json.Unmarshal(data, &module); err != nil {
		return nil, fmt.Errorf("failed to unmarshal module %s: %w", foundPath, err)
	}

	return &module, nil
}

func (c *Composer) channelsToRegisters(
	module *types.ModuleDefinition,
	prefix string,
	inputOffset int,
	outputOffset int,
) []types.RegisterDefinition {
	registers := make([]types.RegisterDefinition, 0, len(module.Channels))

	for _, channel := range module.Channels {
		reg := c.channelToRegister(channel, prefix, inputOffset, outputOffset)
		registers = append(registers, reg)
	}

	return registers
}

func (c *Composer) channelToRegister(
	channel types.ChannelInfo,
	prefix string,
	inputOffset int,
	outputOffset int,
) types.RegisterDefinition {
	fullName := fmt.Sprintf("%s.%s", prefix, channel.Name)

	var regType types.RegisterType
	var address uint16
	var access types.AccessType

	switch channel.Type {
	case "digital_input":
		regType = types.RegisterTypeInputRegister
		address = uint16(inputOffset)
		access = types.AccessTypeReadOnly

	case "digital_output":
		regType = types.RegisterTypeHoldingRegister
		address = uint16(outputOffset)
		access = types.AccessTypeReadWrite

	case "analog_input":
		regType = types.RegisterTypeInputRegister
		address = uint16(inputOffset + (channel.ID * 2)) // 2 bytes per analog
		access = types.AccessTypeReadOnly

	case "analog_output":
		regType = types.RegisterTypeHoldingRegister
		address = uint16(outputOffset + (channel.ID * 2))
		access = types.AccessTypeReadWrite

	default:
		regType = types.RegisterTypeInputRegister
		address = 0
		access = types.AccessTypeReadOnly
	}

	return types.RegisterDefinition{
		Name:        fullName,
		Address:     address,
		Type:        regType,
		DataType:    types.DataTypeBool, // Default for digital I/O
		ScaleFactor: 1.0,
		Access:      access,
		Description: fmt.Sprintf("%s (bit %d)", channel.Description, channel.BitOffset),
	}
}

func (c *Composer) createRegisterGroups(registers []types.RegisterDefinition) []types.RegisterGroup {
	groups := make([]types.RegisterGroup, 0)

	// Group 1: Fast polling for I/O (inputs and outputs)
	fastGroup := types.RegisterGroup{
		Name:           "io_fast",
		PollIntervalMs: 20,
		Registers:      make([]string, 0),
	}

	// Group 2: Slow polling for diagnostics
	slowGroup := types.RegisterGroup{
		Name:           "diagnostics",
		PollIntervalMs: 1000,
		Registers:      make([]string, 0),
	}

	for _, reg := range registers {
		// Diagnostics registers (typically high addresses)
		if reg.Address >= 4000 {
			slowGroup.Registers = append(slowGroup.Registers, reg.Name)
		} else {
			// Regular I/O
			fastGroup.Registers = append(fastGroup.Registers, reg.Name)
		}
	}

	if len(fastGroup.Registers) > 0 {
		groups = append(groups, fastGroup)
	}
	if len(slowGroup.Registers) > 0 {
		groups = append(groups, slowGroup)
	}

	return groups
}
