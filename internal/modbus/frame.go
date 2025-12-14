package modbus

import (
	"encoding/binary"
	"fmt"
)

// MBAP Header (7 Bytes) + Function Code + Data
type ModbusFrame struct {
	TransactionID uint16 // 2 Bytes - Request/Response Korrelation
	ProtocolID    uint16 // 2 Bytes - Immer 0x0000 für Modbus
	Length        uint16 // 2 Bytes - Anzahl folgender Bytes
	UnitID        uint8  // 1 Byte - Slave Address
	FunctionCode  uint8  // 1 Byte - Modbus Function
	Data          []byte // Variable Länge
}

// Modbus Function Codes
const (
	FuncCodeReadCoils              = 0x01
	FuncCodeReadDiscreteInputs     = 0x02
	FuncCodeReadHoldingRegisters   = 0x03
	FuncCodeReadInputRegisters     = 0x04
	FuncCodeWriteSingleCoil        = 0x05
	FuncCodeWriteSingleRegister    = 0x06
	FuncCodeWriteMultipleCoils     = 0x0F
	FuncCodeWriteMultipleRegisters = 0x10
)

// Encode erstellt das komplette TCP Frame
func (f *ModbusFrame) Encode() []byte {
	// PDU Length = Function Code (1) + Data
	f.Length = uint16(len(f.Data) + 2) // +2 für UnitID + FunctionCode
	
	frame := make([]byte, 7+len(f.Data)+1) // MBAP(7) + FuncCode(1) + Data
	
	// MBAP Header
	binary.BigEndian.PutUint16(frame[0:2], f.TransactionID)
	binary.BigEndian.PutUint16(frame[2:4], f.ProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], f.Length)
	frame[6] = f.UnitID
	
	// PDU
	frame[7] = f.FunctionCode
	copy(frame[8:], f.Data)
	
	return frame
}

// Decode parst ein empfangenes Frame
func DecodeFrame(data []byte) (*ModbusFrame, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("frame too short: %d bytes", len(data))
	}
	
	frame := &ModbusFrame{
		TransactionID: binary.BigEndian.Uint16(data[0:2]),
		ProtocolID:    binary.BigEndian.Uint16(data[2:4]),
		Length:        binary.BigEndian.Uint16(data[4:6]),
		UnitID:        data[6],
		FunctionCode:  data[7],
	}
	
	// Validate Protocol ID
	if frame.ProtocolID != 0x0000 {
		return nil, fmt.Errorf("invalid protocol ID: 0x%04X", frame.ProtocolID)
	}
	
	// Data extrahieren
	if len(data) > 8 {
		frame.Data = data[8:]
	}
	
	return frame, nil
}

// ReadHoldingRegistersRequest erstellt Request für Function Code 0x03
func ReadHoldingRegistersRequest(transactionID uint16, unitID uint8, startAddr uint16, quantity uint16) *ModbusFrame {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], startAddr)
	binary.BigEndian.PutUint16(data[2:4], quantity)
	
	return &ModbusFrame{
		TransactionID: transactionID,
		ProtocolID:    0x0000,
		UnitID:        unitID,
		FunctionCode:  FuncCodeReadHoldingRegisters,
		Data:          data,
	}
}

// WriteSingleRegisterRequest erstellt Request für Function Code 0x06
func WriteSingleRegisterRequest(transactionID uint16, unitID uint8, addr uint16, value uint16) *ModbusFrame {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], addr)
	binary.BigEndian.PutUint16(data[2:4], value)
	
	return &ModbusFrame{
		TransactionID: transactionID,
		ProtocolID:    0x0000,
		UnitID:        unitID,
		FunctionCode:  FuncCodeWriteSingleRegister,
		Data:          data,
	}
}

// ParseRegisterResponse parst Holding/Input Register Response
func (f *ModbusFrame) ParseRegisterResponse() ([]uint16, error) {
	if len(f.Data) < 1 {
		return nil, fmt.Errorf("response too short")
	}
	
	byteCount := f.Data[0]
	if len(f.Data) < int(byteCount)+1 {
		return nil, fmt.Errorf("incomplete response data")
	}
	
	registerCount := byteCount / 2
	registers := make([]uint16, registerCount)
	
	for i := 0; i < int(registerCount); i++ {
		offset := 1 + (i * 2)
		registers[i] = binary.BigEndian.Uint16(f.Data[offset : offset+2])
	}
	
	return registers, nil
}
