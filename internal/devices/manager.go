package devices

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/modbus"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Manager struct {
	loader      *ProfileLoader
	devices     map[uuid.UUID]*modbus.Device
	pollers     map[uuid.UUID]*modbus.Poller
	mu          sync.RWMutex
	logger      *zap.Logger
}

func NewManager(searchPaths []string, logger *zap.Logger) (*Manager, error) {
	loader, err := NewProfileLoader(searchPaths)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile loader: %w", err)
	}

	return &Manager{
		loader:  loader,
		devices: make(map[uuid.UUID]*modbus.Device),
		pollers: make(map[uuid.UUID]*modbus.Poller),
		logger:  logger,
	}, nil
}

// LoadDevice lädt Device Profile und erstellt Device-Instanz
func (m *Manager) LoadDevice(
	name string,
	profilePath string,
	ipAddress string,
	port int,
	unitID uint8,
	ioMapping map[string]string,
	timeout time.Duration,
) (*modbus.Device, error) {
	// Profile laden (lazy)
	profile, err := m.loader.Load(profilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load profile %s: %w", profilePath, err)
	}

	// Device erstellen
	device, err := modbus.NewDevice(name, ipAddress, port, unitID, profile, ioMapping, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create device: %w", err)
	}

	// Verbinden
	if err := device.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect device: %w", err)
	}

	m.mu.Lock()
	m.devices[device.ID] = device
	m.mu.Unlock()

	m.logger.Info("Device loaded",
		zap.String("name", name),
		zap.String("profile", profilePath),
		zap.String("address", ipAddress))

	return device, nil
}

// StartPoller startet Poller für ein Device
func (m *Manager) StartPoller(deviceID uuid.UUID, interval time.Duration) error {
	m.mu.RLock()
	device, exists := m.devices[deviceID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	poller := modbus.NewPoller(device, interval, m.logger)
	if err := poller.Start(); err != nil {
		return fmt.Errorf("failed to start poller: %w", err)
	}

	m.mu.Lock()
	m.pollers[deviceID] = poller
	m.mu.Unlock()

	return nil
}

// GetDevice gibt Device zurück
func (m *Manager) GetDevice(deviceID uuid.UUID) (*modbus.Device, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	device, exists := m.devices[deviceID]
	return device, exists
}

// GetDeviceByName gibt Device nach Name zurück
func (m *Manager) GetDeviceByName(name string) (*modbus.Device, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, device := range m.devices {
		if device.Name == name {
			return device, true
		}
	}

	return nil, false
}

// StopAll stoppt alle Poller und trennt alle Devices
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Alle Poller stoppen
	for _, poller := range m.pollers {
		poller.Stop()
	}

	// Alle Devices trennen
	for _, device := range m.devices {
		if err := device.Disconnect(); err != nil {
			m.logger.Error("Failed to disconnect device",
				zap.String("device", device.Name),
				zap.Error(err))
		}
	}

	return nil
}

// ListDevices gibt alle Devices zurück
func (m *Manager) ListDevices() []*modbus.Device {
	m.mu.RLock()
	defer m.mu.RUnlock()

	devices := make([]*modbus.Device, 0, len(m.devices))
	for _, device := range m.devices {
		devices = append(devices, device)
	}

	return devices
}
