package rest

import (
	"context"
	"fmt"
	"net/http"

	"github.com/KevinKickass/OpenMachineCore/internal/config"
	"github.com/KevinKickass/OpenMachineCore/internal/devices"
	"github.com/KevinKickass/OpenMachineCore/internal/storage"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// LifecycleInterface defines what REST API needs from lifecycle manager
type LifecycleInterface interface {
	GetCurrentStatus() interface{}
	TriggerUpdate(workflowPath string) error
	Shutdown(ctx context.Context) error
	DeviceManager() *devices.Manager
	Storage() *storage.PostgresClient
	Config() *config.Config
}

type Server struct {
	router    *gin.Engine
	server    *http.Server
	lifecycle LifecycleInterface
	logger    *zap.Logger
}

func NewServer(cfg *config.Config, lifecycle LifecycleInterface, logger *zap.Logger) *Server {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)
	
	router := gin.New()
	
	// Middleware
	router.Use(gin.Recovery())
	router.Use(LoggerMiddleware(logger))
	router.Use(CORSMiddleware())
	
	s := &Server{
		router:    router,
		lifecycle: lifecycle,
		logger:    logger,
	}
	
	// Setup routes
	s.setupRoutes()
	
	// Create HTTP server
	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler: router,
	}
	
	return s
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
		}
		
		// Module endpoints (for UI)
		modules := v1.Group("/modules")
		{
			modules.GET("", s.listModules)
			modules.GET("/:vendor/:model", s.getModule)
		}
	}
}

func (s *Server) Start() error {
	s.logger.Info("Starting REST API server", zap.String("addr", s.server.Addr))
	
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("REST API server error", zap.Error(err))
		}
	}()
	
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down REST API server")
	return s.server.Shutdown(ctx)
}

// Health check
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": c.GetInt64("timestamp"),
	})
}
