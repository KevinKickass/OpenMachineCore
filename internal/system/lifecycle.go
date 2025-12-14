package system

import (
    "context"
    "fmt"
    "net"
    "sync"
    "time"

    "github.com/KevinKickass/OpenMachineCore/internal/api/rest"
    "github.com/KevinKickass/OpenMachineCore/internal/config"
    "github.com/KevinKickass/OpenMachineCore/internal/devices"
    "github.com/KevinKickass/OpenMachineCore/internal/interfaces"
    "github.com/KevinKickass/OpenMachineCore/internal/storage"
    "github.com/KevinKickass/OpenMachineCore/internal/workflow/engine"
    "github.com/KevinKickass/OpenMachineCore/internal/workflow/executor"
    "github.com/KevinKickass/OpenMachineCore/internal/workflow/streaming"
    "github.com/KevinKickass/OpenMachineCore/internal/machine"
    pb "github.com/KevinKickass/OpenMachineCore/api/proto"
    "go.uber.org/zap"
    "google.golang.org/grpc"
)

// ALLE Type Definitionen (SystemState, UpdateProgress, SystemStatus) ENTFERNEN
// Diese sind jetzt in state.go

type LifecycleManager struct {
    config            *config.Config
    storage           *storage.PostgresClient
    deviceManager     *devices.Manager
    workflowEngine    *engine.Engine
    eventStreamer     *streaming.EventStreamer
    workflowService   *streaming.WorkflowService
    machineController *machine.Controller
    logger            *zap.Logger
    
    restServer   *rest.Server
    grpcServer   *grpc.Server
    
    stateMu         sync.RWMutex
    currentState    SystemState
    updateProgress  UpdateProgress
    
    listenersMu     sync.RWMutex
    statusListeners []chan SystemStatus
    
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

    // Initialize Workflow Engine components
    eventStreamer := streaming.NewEventStreamer()
    stepExecutor := executor.NewStepExecutor(deviceManager, storage)
    workflowEngine := engine.NewEngine(storage, stepExecutor, eventStreamer, logger)
    workflowService := streaming.NewWorkflowService(eventStreamer)

    // Initialize Machine Controller
    machineController := machine.NewController(logger, workflowEngine, storage)

    return &LifecycleManager{
        config:            cfg,
        storage:           storage,
        deviceManager:     deviceManager,
        workflowEngine:    workflowEngine,
        eventStreamer:     eventStreamer,
        workflowService:   workflowService,
        machineController: machineController,
        logger:            logger,
        currentState:      StateInitializing,
        shutdownChan:      make(chan struct{}),
        statusListeners:   make([]chan SystemStatus, 0),
    }
}

// MachineController returns the machine controller
func (lm *LifecycleManager) MachineController() *machine.Controller {
    return lm.machineController
}

// Start starts the entire system
func (lm *LifecycleManager) Start() error {
    lm.logger.Info("Starting OpenMachineCore with Workflow Engine")

    // State: Initializing
    lm.setState(StateInitializing)
    lm.broadcastStatus()

    // Load devices from database
    if err := lm.loadDevicesFromDB(); err != nil {
        lm.logger.Warn("Failed to load devices from database", zap.Error(err))
        // Continue anyway, not critical
    }

    // Start gRPC Server (with Workflow Service)
    if err := lm.startGRPCServer(); err != nil {
        lm.setError(fmt.Errorf("failed to start gRPC: %w", err))
        return err
    }

    // Start REST API Server
    if err := lm.startRESTServer(); err != nil {
        lm.setError(fmt.Errorf("failed to start REST API: %w", err))
        return err
    }

    // State: Running
    lm.setState(StateRunning)
    lm.broadcastStatus()

    lm.logger.Info("System started successfully",
        zap.Int("grpc_port", lm.config.Server.GRPCPort),
        zap.Int("http_port", lm.config.Server.HTTPPort),
        zap.Bool("workflow_engine_enabled", true))

    return nil
}

func (lm *LifecycleManager) loadDevicesFromDB() error {
    ctx := context.Background()
    
    compositions, err := lm.storage.LoadAllDeviceCompositions(ctx)
    if err != nil {
        return fmt.Errorf("failed to load compositions: %w", err)
    }

    lm.logger.Info("Loading devices from database", zap.Int("count", len(compositions)))

    timeout := time.Duration(lm.config.Modbus.DefaultTimeout)

    for _, comp := range compositions {
        device, err := lm.deviceManager.LoadDeviceFromComposition(comp, timeout)
        if err != nil {
            lm.logger.Error("Failed to load device",
                zap.String("instance_id", comp.InstanceID),
                zap.Error(err))
            continue
        }

        // Start poller for this device
        pollInterval := time.Duration(lm.config.Modbus.DefaultPollInterval)
        if err := lm.deviceManager.StartPoller(device.ID, pollInterval); err != nil {
            lm.logger.Error("Failed to start poller",
                zap.String("instance_id", comp.InstanceID),
                zap.Error(err))
        }

        lm.logger.Info("Device loaded and poller started",
            zap.String("instance_id", comp.InstanceID))
    }

    return nil
}

// Shutdown gracefully shuts down the system
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
    errChan := make(chan error, 4)

    // 1. Stop Device Manager (all pollers & connections)
    wg.Add(1)
    go func() {
        defer wg.Done()
        if err := lm.deviceManager.StopAll(ctx); err != nil {
            errChan <- fmt.Errorf("device manager stop failed: %w", err)
        }
    }()

    // 2. REST API Server graceful shutdown
    if lm.restServer != nil {
        wg.Add(1)
        go func() {
            defer wg.Done()
            shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
            defer cancel()

            if err := lm.restServer.Shutdown(shutdownCtx); err != nil {
                errChan <- fmt.Errorf("rest api shutdown failed: %w", err)
            }
        }()
    }

    // 3. gRPC Server graceful stop
    if lm.grpcServer != nil {
        wg.Add(1)
        go func() {
            defer wg.Done()
            lm.logger.Info("Stopping gRPC server (including Workflow Service)")
            lm.grpcServer.GracefulStop()
        }()
    }

    // Wait for all shutdowns
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

    // Register Workflow Service
    pb.RegisterWorkflowServiceServer(lm.grpcServer, lm.workflowService)
    lm.logger.Info("Workflow gRPC service registered")

    go func() {
        lm.logger.Info("gRPC server listening", 
            zap.Int("port", lm.config.Server.GRPCPort),
            zap.String("services", "WorkflowService"))
        if err := lm.grpcServer.Serve(lis); err != nil {
            lm.logger.Error("gRPC server failed", zap.Error(err))
        }
    }()

    return nil
}

func (lm *LifecycleManager) startRESTServer() error {
    lm.restServer = rest.NewServer(lm.config, lm, lm.logger)
    return lm.restServer.Start()
}

// TriggerUpdate initiates system update
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
    time.Sleep(2 * time.Second) // Simulate work

    // Phase 3: Initializing devices (70%)
    lm.setUpdateProgress("Initializing devices", 70, "Connecting to devices")
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

// GetCurrentStatus returns current system status (Interface implementation)
func (lm *LifecycleManager) GetCurrentStatus() interfaces.SystemStatus {
    lm.stateMu.RLock()
    defer lm.stateMu.RUnlock()

    devices := lm.deviceManager.ListDevices()
    connected := 0
    for _, d := range devices {
        if d.Client != nil {
            connected++
        }
    }

    return interfaces.SystemStatus{
        State:            lm.currentState.String(),
        DeviceCount:      len(devices),
        ConnectedDevices: connected,
    }
}

// GetCurrentStatusDetailed returns detailed status with update progress
func (lm *LifecycleManager) GetCurrentStatusDetailed() interface{} {
    lm.stateMu.RLock()
    defer lm.stateMu.RUnlock()

    return map[string]interface{}{
        "state": lm.currentState.String(),
        "update_progress": map[string]interface{}{
            "phase":      lm.updateProgress.Phase,
            "progress":   lm.updateProgress.Progress,
            "message":    lm.updateProgress.Message,
            "started_at": lm.updateProgress.StartedAt,
        },
        "timestamp": time.Now().Unix(),
    }
}

// getStatusInternal returns typed status (for internal use)
func (lm *LifecycleManager) getStatusInternal() SystemStatus {
    lm.stateMu.RLock()
    defer lm.stateMu.RUnlock()

    return SystemStatus{
        State:          lm.currentState,
        UpdateProgress: lm.updateProgress,
        Timestamp:      time.Now().Unix(),
    }
}

func (lm *LifecycleManager) broadcastStatus() {
    status := lm.getStatusInternal()

    lm.listenersMu.RLock()
    defer lm.listenersMu.RUnlock()

    for _, listener := range lm.statusListeners {
        select {
        case listener <- status:
        default:
            // Channel full, skip
        }
    }
}

// SubscribeStatus subscribes to status updates
func (lm *LifecycleManager) SubscribeStatus() chan SystemStatus {
    ch := make(chan SystemStatus, 10)

    lm.listenersMu.Lock()
    lm.statusListeners = append(lm.statusListeners, ch)
    lm.listenersMu.Unlock()

    return ch
}

// UnsubscribeStatus unsubscribes from status updates
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

// DeviceManager returns the device manager
func (lm *LifecycleManager) DeviceManager() *devices.Manager {
    return lm.deviceManager
}

// Storage returns the storage client
func (lm *LifecycleManager) Storage() *storage.PostgresClient {
    return lm.storage
}

// Config returns the configuration
func (lm *LifecycleManager) Config() *config.Config {
    return lm.config
}

// WorkflowEngine returns the workflow engine
func (lm *LifecycleManager) WorkflowEngine() *engine.Engine {
    return lm.workflowEngine
}
