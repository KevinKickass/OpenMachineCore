package system

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/config"
	"github.com/KevinKickass/OpenMachineCore/internal/devices"
	"github.com/KevinKickass/OpenMachineCore/internal/storage"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type LifecycleManager struct {
	config         *config.Config
	storage        *storage.PostgresClient
	deviceManager  *devices.Manager
	logger         *zap.Logger

	// Servers
	httpServer *http.Server
	grpcServer *grpc.Server

	// State
	currentState   SystemState
	stateMu        sync.RWMutex
	updateProgress UpdateProgress

	// Status Broadcasting
	statusListeners []chan SystemStatus
	listenersMu     sync.RWMutex

	// Shutdown
	shutdownChan chan struct{}
	shutdownOnce sync.Once
}

func NewLifecycleManager(
	storage *storage.PostgresClient,
	cfg *config.Config,
	logger *zap.Logger,
) *LifecycleManager {
	deviceManager, err := devices.NewManager(cfg.Devices.SearchPaths, logger)
	if err != nil {
		logger.Fatal("Failed to create device manager", zap.Error(err))
	}

	return &LifecycleManager{
		config:          cfg,
		storage:         storage,
		deviceManager:   deviceManager,
		logger:          logger,
		currentState:    StateInitializing,
		shutdownChan:    make(chan struct{}),
		statusListeners: make([]chan SystemStatus, 0),
	}
}

// Start startet das gesamte System
func (lm *LifecycleManager) Start() error {
	lm.logger.Info("Starting OpenMachineCore")

	// State: Initializing
	lm.setState(StateInitializing)
	lm.broadcastStatus()

	// gRPC Server starten
	if err := lm.startGRPCServer(); err != nil {
		lm.setError(fmt.Errorf("failed to start gRPC: %w", err))
		return err
	}

	// HTTP Server starten
	if err := lm.startHTTPServer(); err != nil {
		lm.setError(fmt.Errorf("failed to start HTTP: %w", err))
		return err
	}

	// State: Running
	lm.setState(StateRunning)
	lm.broadcastStatus()

	lm.logger.Info("System started successfully",
		zap.Int("grpc_port", lm.config.Server.GRPCPort),
		zap.Int("http_port", lm.config.Server.HTTPPort))

	return nil
}

// Shutdown fährt das System herunter
func (lm *LifecycleManager) Shutdown(ctx context.Context) error {
	var shutdownErr error

	lm.shutdownOnce.Do(func() {
		lm.logger.Info("Shutting down system")

		lm.setState(StateStopping)
		lm.broadcastStatus()

		shutdownErr = lm.gracefulShutdown(ctx)

		lm.setState(StateStopped)
		lm.broadcastStatus()

		close(lm.shutdownChan)
	})

	return shutdownErr
}

func (lm *LifecycleManager) gracefulShutdown(ctx context.Context) error {
	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	// 1. Stoppe Device Manager (alle Poller & Connections)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := lm.deviceManager.StopAll(ctx); err != nil {
			errChan <- fmt.Errorf("device manager stop failed: %w", err)
		}
	}()

	// 2. HTTP Server graceful shutdown
	if lm.httpServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			if err := lm.httpServer.Shutdown(shutdownCtx); err != nil {
				errChan <- fmt.Errorf("http shutdown failed: %w", err)
			}
		}()
	}

	// 3. gRPC Server graceful stop
	if lm.grpcServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.grpcServer.GracefulStop()
		}()
	}

	// Warte auf alle Shutdowns
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		lm.logger.Info("Graceful shutdown completed")
		return nil
	case <-ctx.Done():
		lm.logger.Warn("Shutdown timeout, forcing stop")
		return fmt.Errorf("shutdown timeout exceeded")
	case err := <-errChan:
		return err
	}
}

func (lm *LifecycleManager) startGRPCServer() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", lm.config.Server.GRPCPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	lm.grpcServer = grpc.NewServer()

	// gRPC Services werden hier registriert (später)
	// pb.RegisterSystemServiceServer(lm.grpcServer, &grpcService{lm: lm})
	// pb.RegisterMachineServiceServer(lm.grpcServer, &machineService{lm: lm})

	go func() {
		lm.logger.Info("gRPC server listening", zap.Int("port", lm.config.Server.GRPCPort))
		if err := lm.grpcServer.Serve(lis); err != nil {
			lm.logger.Error("gRPC server failed", zap.Error(err))
		}
	}()

	return nil
}

func (lm *LifecycleManager) startHTTPServer() error {
	// HTTP Router wird hier erstellt (später mit Gin)
	// router := gin.Default()
	// lm.setupRESTRoutes(router)

	lm.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", lm.config.Server.HTTPPort),
		Handler: nil, // TODO: Gin Router
	}

	go func() {
		lm.logger.Info("HTTP server listening", zap.Int("port", lm.config.Server.HTTPPort))
		if err := lm.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			lm.logger.Error("HTTP server failed", zap.Error(err))
		}
	}()

	return nil
}

// TriggerUpdate initiiert System-Update
func (lm *LifecycleManager) TriggerUpdate(workflowPath string) error {
	lm.stateMu.Lock()
	if lm.currentState != StateRunning {
		lm.stateMu.Unlock()
		return fmt.Errorf("cannot update: system not in running state")
	}
	lm.currentState = StateUpdating
	lm.stateMu.Unlock()

	lm.broadcastStatus()

	go lm.executeUpdate(workflowPath)
	return nil
}

func (lm *LifecycleManager) executeUpdate(workflowPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Phase 1: Stopping services (15%)
	lm.setUpdateProgress("Stopping services", 5, "Gracefully stopping all services")
	if err := lm.gracefulShutdown(ctx); err != nil {
		lm.handleUpdateError(err)
		return
	}

	// Phase 2: Loading workflow (50%)
	lm.setUpdateProgress("Loading workflow", 50, fmt.Sprintf("Loading workflow from %s", workflowPath))
	// TODO: Workflow laden

	time.Sleep(2 * time.Second) // Simuliere Arbeit

	// Phase 3: Initializing devices (70%)
	lm.setUpdateProgress("Initializing devices", 70, "Connecting to devices")
	// TODO: Devices initialisieren

	time.Sleep(2 * time.Second)

	// Phase 4: Starting services (95%)
	lm.setUpdateProgress("Starting services", 95, "Restarting all services")
	if err := lm.Start(); err != nil {
		lm.handleUpdateError(err)
		return
	}

	// Phase 5: Complete (100%)
	lm.setUpdateProgress("Complete", 100, "Update completed successfully")

	lm.setState(StateRunning)
	lm.broadcastStatus()

	lm.logger.Info("Update completed successfully")
}

func (lm *LifecycleManager) handleUpdateError(err error) {
	lm.logger.Error("Update failed", zap.Error(err))
	lm.setError(err)
	lm.broadcastStatus()
}

func (lm *LifecycleManager) setState(state SystemState) {
	lm.stateMu.Lock()
	defer lm.stateMu.Unlock()
	lm.currentState = state
}

func (lm *LifecycleManager) setError(err error) {
	lm.stateMu.Lock()
	defer lm.stateMu.Unlock()
	lm.currentState = StateError
}

func (lm *LifecycleManager) setUpdateProgress(phase string, progress int, message string) {
	lm.stateMu.Lock()
	lm.updateProgress = UpdateProgress{
		Phase:     phase,
		Progress:  progress,
		Message:   message,
		StartedAt: time.Now().Unix(),
	}
	lm.stateMu.Unlock()

	lm.broadcastStatus()
}

// GetCurrentStatus gibt aktuellen Status zurück
func (lm *LifecycleManager) GetCurrentStatus() SystemStatus {
	lm.stateMu.RLock()
	defer lm.stateMu.RUnlock()

	return SystemStatus{
		State:          lm.currentState,
		UpdateProgress: lm.updateProgress,
		Timestamp:      time.Now().Unix(),
	}
}

// SubscribeStatus abonniert Status-Updates
func (lm *LifecycleManager) SubscribeStatus() chan SystemStatus {
	ch := make(chan SystemStatus, 10)

	lm.listenersMu.Lock()
	lm.statusListeners = append(lm.statusListeners, ch)
	lm.listenersMu.Unlock()

	return ch
}

// UnsubscribeStatus beendet Abo
func (lm *LifecycleManager) UnsubscribeStatus(ch chan SystemStatus) {
	lm.listenersMu.Lock()
	defer lm.listenersMu.Unlock()

	for i, listener := range lm.statusListeners {
		if listener == ch {
			lm.statusListeners = append(lm.statusListeners[:i], lm.statusListeners[i+1:]...)
			close(ch)
			break
		}
	}
}

func (lm *LifecycleManager) broadcastStatus() {
	status := lm.GetCurrentStatus()

	lm.listenersMu.RLock()
	defer lm.listenersMu.RUnlock()

	for _, listener := range lm.statusListeners {
		select {
		case listener <- status:
		default:
			// Channel voll, Skip
		}
	}
}

// DeviceManager gibt Device Manager zurück
func (lm *LifecycleManager) DeviceManager() *devices.Manager {
	return lm.deviceManager
}
