package types

import (
	"github.com/google/uuid"
)

type DeviceProfileDefinition struct {
	DeviceProfile DeviceProfileInfo    `json:"device_profile"`
	Connection    ConnectionConfig     `json:"connection"`
	Registers     []RegisterDefinition `json:"registers"`
	Groups        []RegisterGroup      `json:"register_groups,omitempty"`
}

type DeviceProfileInfo struct {
	ID          string `json:"id"`
	Vendor      string `json:"vendor"`
	Model       string `json:"model"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type ConnectionConfig struct {
	Protocol       string `json:"protocol"`
	Port           int    `json:"port"`
	UnitID         int    `json:"unit_id"`
	PollIntervalMs int    `json:"poll_interval_ms"`
	TimeoutMs      int    `json:"timeout_ms"`
}

type RegisterDefinition struct {
	Name        string       `json:"name"`
	Address     uint16       `json:"address"`
	Type        RegisterType `json:"type"`
	DataType    DataType     `json:"data_type"`
	ScaleFactor float64      `json:"scale_factor"`
	Unit        string       `json:"unit"`
	Access      AccessType   `json:"access"`
	Description string       `json:"description"`
}

type RegisterGroup struct {
	Name           string   `json:"name"`
	PollIntervalMs int      `json:"poll_interval_ms"`
	Registers      []string `json:"registers"`
}

type RegisterType string

const (
	RegisterTypeCoil            RegisterType = "coil"
	RegisterTypeDiscreteInput   RegisterType = "discrete_input"
	RegisterTypeInputRegister   RegisterType = "input_register"
	RegisterTypeHoldingRegister RegisterType = "holding_register"
)

type DataType string

const (
	DataTypeBool    DataType = "bool"
	DataTypeInt16   DataType = "int16"
	DataTypeUint16  DataType = "uint16"
	DataTypeInt32   DataType = "int32"
	DataTypeUint32  DataType = "uint32"
	DataTypeFloat32 DataType = "float32"
	DataTypeFloat64 DataType = "float64"
)

type AccessType string

const (
	AccessTypeReadOnly  AccessType = "read_only"
	AccessTypeReadWrite AccessType = "read_write"
)

// Device Runtime Info
type DeviceInfo struct {
	ID        uuid.UUID
	Name      string
	IPAddress string
	Port      int
	UnitID    uint8
	Connected bool
}
