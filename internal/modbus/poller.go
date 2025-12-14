package modbus

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

type Poller struct {
	device       *Device
	interval     time.Duration
	logger       *zap.Logger
	stopChan     chan struct{}
	wg           sync.WaitGroup
	running      bool
	mu           sync.Mutex
}

func NewPoller(device *Device, interval time.Duration, logger *zap.Logger) *Poller {
	return &Poller{
		device:   device,
		interval: interval,
		logger:   logger,
		stopChan: make(chan struct{}),
	}
}

// Start startet das zyklische Polling
func (p *Poller) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.running = true
	p.wg.Add(1)

	go p.pollLoop()

	p.logger.Info("Poller started",
		zap.String("device", p.device.Name),
		zap.Duration("interval", p.interval))

	return nil
}

// Stop stoppt das Polling
func (p *Poller) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	close(p.stopChan)
	p.wg.Wait()

	p.mu.Lock()
	p.running = false
	p.mu.Unlock()

	p.logger.Info("Poller stopped", zap.String("device", p.device.Name))
}

func (p *Poller) pollLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.pollDevice()
		}
	}
}

func (p *Poller) pollDevice() {
	ctx, cancel := context.WithTimeout(context.Background(), p.interval/2)
	defer cancel()

	// Alle Register im Profile pollen
	for _, reg := range p.device.Profile.Registers {
		if reg.Access == "read_only" || reg.Access == "read_write" {
			_, err := p.device.ReadRegister(ctx, reg.Name)
			if err != nil {
				p.logger.Error("Poll failed",
					zap.String("device", p.device.Name),
					zap.String("register", reg.Name),
					zap.Error(err))
			}
		}
	}
}

// IsRunning gibt an ob Poller läuft
func (p *Poller) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}
