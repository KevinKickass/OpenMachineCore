package rest

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/KevinKickass/OpenMachineCore/internal/config"
    "github.com/KevinKickass/OpenMachineCore/internal/interfaces"  // Verwenden
    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
)

type Server struct {
    router *gin.Engine
    lm     interfaces.LifecycleManager  // Interface verwenden
    logger *zap.Logger
    server *http.Server
}

func NewServer(cfg *config.Config, lm interfaces.LifecycleManager, logger *zap.Logger) *Server {
    gin.SetMode(gin.ReleaseMode)
    
    s := &Server{
        router: gin.Default(),
        lm:     lm,
        logger: logger,
    }

    s.setupRoutes()
    
    s.server = &http.Server{
        Addr:         fmt.Sprintf(":%d", cfg.Server.HTTPPort),
        Handler:      s.router,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    return s
}

func (s *Server) Start() error {
    s.logger.Info("Starting REST API server", zap.String("address", s.server.Addr))
    
    go func() {
        if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            s.logger.Fatal("REST server failed", zap.Error(err))
        }
    }()
    
    return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
    s.logger.Info("Shutting down REST API server")
    return s.server.Shutdown(ctx)
}

func (s *Server) setupRoutes() {
    // Health check
    s.router.GET("/health", s.healthCheck)
    
    // API v1
    v1 := s.router.Group("/api/v1")
    {
        // System endpoints
        system := v1.Group("/system")
        {
            system.GET("/status", s.getSystemStatus)
            system.POST("/update", s.triggerUpdate)
            system.POST("/shutdown", s.shutdown)
        }
        
        // Device endpoints
        devices := v1.Group("/devices")
        {
            devices.GET("", s.listDevices)
            devices.GET("/:id", s.getDevice)
            devices.POST("", s.createDevice)
            devices.DELETE("/:id", s.deleteDevice)
            devices.POST("/:id/read", s.readRegister)
            devices.POST("/:id/write", s.writeRegister)
        }
        
        // Workflow endpoints
        workflows := v1.Group("/workflows")
        {
            workflows.GET("", s.listWorkflows)
            workflows.GET("/:id", s.getWorkflow)
            workflows.POST("", s.createWorkflow)
            workflows.PUT("/:id", s.updateWorkflow)
            workflows.DELETE("/:id", s.deleteWorkflow)
            workflows.POST("/:id/activate", s.activateWorkflow)
            workflows.POST("/:id/execute", s.executeWorkflow)
        }
        
        // Workflow execution endpoints
        executions := v1.Group("/executions")
        {
            executions.GET("/:id", s.getExecutionStatus)
            executions.GET("/:id/steps", s.getExecutionSteps)
        }
        
        // Module endpoints (device descriptors)
        modules := v1.Group("/modules")
        {
            modules.GET("", s.listModules)
            modules.GET("/:vendor", s.getVendorModules)
            modules.GET("/:vendor/:model", s.getModule)
        }

		// Machine control endpoints
		machine := v1.Group("/machine")
		{
			machine.GET("/status", s.getMachineStatus)
			machine.POST("/command", s.executeMachineCommand)
			machine.POST("/configure", s.configureMachineWorkflows)
		}
    }
}

// Health check
func (s *Server) healthCheck(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "status":    "ok",
        "timestamp": c.GetInt64("timestamp"),
    })
}
