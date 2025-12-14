package modbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/types"
	"github.com/google/uuid"
)

type Device struct {
	ID           uuid.UUID
	Name         string
	Profile      *types.DeviceProfileDefinition
	Client       *Client
	IOMapping    map[string]string // logicalName -> registerName
	RegisterMap  map[string]*types.RegisterDefinition
	mu           sync.RWMutex
	lastValues   map[string]interface{}
	connected    bool
}

func NewDevice(
	name string,
	ipAddress string,
	port int,
	unitID uint8,
	profile *types.DeviceProfileDefinition,
	ioMapping map[string]string,
	timeout time.Duration,
) (*Device, error) {
	registerMap := make(map[string]*types.RegisterDefinition)
	for i := range profile.Registers {
		reg := &profile.Registers[i]
		registerMap[reg.Name] = reg
	}

	address := fmt.Sprintf("%s:%d", ipAddress, port)
	client := NewClient(address, timeout)

	return &Device{
		ID:          uuid.New(),
		Name:        name,
		Profile:     profile,
		Client:      client,
		IOMapping:   ioMapping,
		RegisterMap: registerMap,
		lastValues:  make(map[string]interface{}),
		connected:   false,
	}, nil
}

func (d *Device) Connect() error {
	if err := d.Client.Connect(); err != nil {
		return fmt.Errorf("failed to connect to %s: %w", d.Name, err)
	}

	d.mu.Lock()
	d.connected = true
	d.mu.Unlock()

	return nil
}

func (d *Device) Disconnect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.connected {
		return nil
	}

	if err := d.Client.Close(); err != nil {
		return err
	}

	d.connected = false
	return nil
}

// ReadRegister liest einen Register nach Name
func (d *Device) ReadRegister(ctx context.Context, registerName string) (interface{}, error) {
	d.mu.RLock()
	reg, exists := d.RegisterMap[registerName]
	d.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("register not found: %s", registerName)
	}

	// Support for Coils and Discrete Inputs
	if reg.Type == types.RegisterTypeCoil || reg.Type == types.RegisterTypeDiscreteInput {
		// For single bit, read as coil/discrete input
		// TODO: Implement ReadCoils/ReadDiscreteInputs
		return nil, fmt.Errorf("coil/discrete input reading not yet implemented")
	}

	// For registers (holding/input)
	if reg.Type != types.RegisterTypeHoldingRegister && reg.Type != types.RegisterTypeInputRegister {
		return nil, fmt.Errorf("unsupported register type: %s", reg.Type)
	}

	quantity := d.getRegisterQuantity(reg.DataType)

	// Modbus Read based on register type
	var values []uint16
	var err error

	if reg.Type == types.RegisterTypeHoldingRegister {
		values, err = d.Client.ReadHoldingRegisters(ctx, uint8(d.Profile.Connection.UnitID), reg.Address, quantity)
	} else {
		values, err = d.Client.ReadInputRegisters(ctx, uint8(d.Profile.Connection.UnitID), reg.Address, quantity)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read register %s: %w", registerName, err)
	}

	// Convert value based on data type
	value := d.convertRegisterValue(values, reg.DataType, reg.ScaleFactor)

	// Cache update
	d.mu.Lock()
	d.lastValues[registerName] = value
	d.mu.Unlock()

	return value, nil
}


// WriteRegister schreibt einen Register
func (d *Device) WriteRegister(ctx context.Context, registerName string, value interface{}) error {
	d.mu.RLock()
	reg, exists := d.RegisterMap[registerName]
	d.mu.RUnlock()

	if !exists {
		return fmt.Errorf("register not found: %s", registerName)
	}

	if reg.Access != types.AccessTypeReadWrite {
		return fmt.Errorf("register %s is read-only", registerName)
	}

	var regValue uint16

	// Convert value to uint16 based on type
	switch v := value.(type) {
	case bool:
		if v {
			regValue = 1
		} else {
			regValue = 0
		}
	case int:
		regValue = uint16(v)
	case int16:
		regValue = uint16(v)
	case uint16:
		regValue = v
	case float64:
		// JSON unmarshals numbers as float64
		if reg.DataType == types.DataTypeBool {
			if v > 0 {
				regValue = 1
			} else {
				regValue = 0
			}
		} else {
			regValue = uint16(v / reg.ScaleFactor)
		}
	default:
		return fmt.Errorf("unsupported value type: %T", value)
	}

	return d.Client.WriteSingleRegister(ctx, uint8(d.Profile.Connection.UnitID), reg.Address, regValue)
}

func (d *Device) ReadLogical(ctx context.Context, logicalName string) (interface{}, error) {
	registerName, exists := d.IOMapping[logicalName]
	if !exists {
		return nil, fmt.Errorf("logical name not mapped: %s", logicalName)
	}

	return d.ReadRegister(ctx, registerName)
}

func (d *Device) WriteLogical(ctx context.Context, logicalName string, value interface{}) error {
	registerName, exists := d.IOMapping[logicalName]
	if !exists {
		return fmt.Errorf("logical name not mapped: %s", logicalName)
	}

	return d.WriteRegister(ctx, registerName, value)
}

func (d *Device) GetLastValue(registerName string) (interface{}, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	value, exists := d.lastValues[registerName]
	return value, exists
}

func (d *Device) getRegisterQuantity(dataType types.DataType) uint16 {
	switch dataType {
	case types.DataTypeBool, types.DataTypeInt16, types.DataTypeUint16:
		return 1
	case types.DataTypeInt32, types.DataTypeUint32, types.DataTypeFloat32:
		return 2
	case types.DataTypeFloat64:
		return 4
	default:
		return 1
	}
}

func (d *Device) convertRegisterValue(registers []uint16, dataType types.DataType, scaleFactor float64) interface{} {
	if scaleFactor == 0 {
		scaleFactor = 1.0
	}

	switch dataType {
	case types.DataTypeBool:
		// For bool, check if any bit is set
		return registers[0] != 0
		
	case types.DataTypeUint16:
		return float64(registers[0]) * scaleFactor
		
	case types.DataTypeInt16:
		return float64(int16(registers[0])) * scaleFactor
		
	case types.DataTypeUint32:
		if len(registers) >= 2 {
			val := uint32(registers[0])<<16 | uint32(registers[1])
			return float64(val) * scaleFactor
		}
		
	case types.DataTypeInt32:
		if len(registers) >= 2 {
			val := int32(registers[0])<<16 | int32(registers[1])
			return float64(val) * scaleFactor
		}
		
	case types.DataTypeFloat32:
		if len(registers) >= 2 {
			// IEEE 754 float32 from 2 registers
			bits := uint32(registers[0])<<16 | uint32(registers[1])
			// TODO: Convert to float32
			return float64(bits)
		}
	}

	return registers[0]
}
