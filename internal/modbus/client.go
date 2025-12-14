package modbus

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

type Client struct {
	address        string
	conn           net.Conn
	mu             sync.Mutex
	transactionID  uint16
	timeout        time.Duration
	connected      bool
}

func NewClient(address string, timeout time.Duration) *Client {
	return &Client{
		address:       address,
		timeout:       timeout,
		transactionID: 0,
	}
}

// Connect stellt TCP-Verbindung her
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.connected {
		return nil
	}
	
	conn, err := net.DialTimeout("tcp", c.address, c.timeout)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	
	c.conn = conn
	c.connected = true
	
	return nil
}

// Close schließt die Verbindung
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if !c.connected {
		return nil
	}
	
	err := c.conn.Close()
	c.connected = false
	c.conn = nil
	
	return err
}

// SendFrame sendet ein Frame und wartet auf Response
func (c *Client) SendFrame(ctx context.Context, request *ModbusFrame) (*ModbusFrame, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}
	
	// Unique Transaction ID
	c.transactionID++
	request.TransactionID = c.transactionID
	
	// Request senden
	requestData := request.Encode()
	
	// Timeout setzen
	deadline := time.Now().Add(c.timeout)
	c.conn.SetWriteDeadline(deadline)
	
	_, err := c.conn.Write(requestData)
	if err != nil {
		return nil, fmt.Errorf("write failed: %w", err)
	}
	
	// Response lesen
	c.conn.SetReadDeadline(deadline)
	
	responseBuffer := make([]byte, 260) // Max Modbus TCP Frame
	n, err := c.conn.Read(responseBuffer)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	
	response, err := DecodeFrame(responseBuffer[:n])
	if err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}
	
	// Transaction ID prüfen
	if response.TransactionID != request.TransactionID {
		return nil, fmt.Errorf("transaction ID mismatch: expected %d, got %d", 
			request.TransactionID, response.TransactionID)
	}
	
	return response, nil
}

// ReadHoldingRegisters liest Holding Registers
func (c *Client) ReadHoldingRegisters(ctx context.Context, unitID uint8, startAddr uint16, quantity uint16) ([]uint16, error) {
	request := ReadHoldingRegistersRequest(0, unitID, startAddr, quantity)
	
	response, err := c.SendFrame(ctx, request)
	if err != nil {
		return nil, err
	}
	
	return response.ParseRegisterResponse()
}

// WriteSingleRegister schreibt ein einzelnes Register
func (c *Client) WriteSingleRegister(ctx context.Context, unitID uint8, addr uint16, value uint16) error {
	request := WriteSingleRegisterRequest(0, unitID, addr, value)
	
	_, err := c.SendFrame(ctx, request)
	return err
}
