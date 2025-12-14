package rest

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/api/websocket"
	"github.com/KevinKickass/OpenMachineCore/internal/auth"
	"github.com/KevinKickass/OpenMachineCore/internal/config"
	"github.com/KevinKickass/OpenMachineCore/internal/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Server struct {
	router      *gin.Engine
	lm          interfaces.LifecycleManager
	logger      *zap.Logger
	server      *http.Server
	wsHub       *websocket.Hub
	authService *auth.AuthService // NEU
}

func NewServer(cfg *config.Config, lm interfaces.LifecycleManager, logger *zap.Logger, wsHub *websocket.Hub, authService *auth.AuthService) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		router:      gin.Default(),
		lm:          lm,
		logger:      logger,
		wsHub:       wsHub,
		authService: authService, // NEU
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
	// Middleware
	s.router.Use(LoggerMiddleware(s.logger))
	s.router.Use(CORSMiddleware())

	// Inject AuthService into Gin context
	s.router.Use(func(c *gin.Context) {
		c.Set("authService", s.authService)
		c.Next()
	})

	// Public routes (no auth required)
	s.router.GET("/health", s.healthCheck)

	// API v1
	v1 := s.router.Group("/api/v1")
	{
		// ==================== AUTH ENDPOINTS (PUBLIC) ====================
		authPublic := v1.Group("/auth")
		{
			authPublic.POST("/login", s.login)
			authPublic.POST("/refresh", s.refreshToken)
		}

		// ==================== AUTH ENDPOINTS (AUTHENTICATED) ====================
		authProtected := v1.Group("/auth")
		authProtected.Use(s.authService.AuthMiddleware())
		{
			authProtected.POST("/logout", s.logout)
			authProtected.GET("/me", s.getCurrentUser)
		}

		// ==================== MACHINE TOKENS (ADMIN ONLY) ====================
		machineTokens := v1.Group("/machine-tokens")
		machineTokens.Use(s.authService.AuthMiddleware())
		machineTokens.Use(auth.RequirePermission(auth.PermAdmin))
		{
			machineTokens.POST("", s.createMachineToken)
			machineTokens.GET("", s.listMachineTokens)
			machineTokens.PATCH("/:id", s.updateMachineToken)
			machineTokens.DELETE("/:id", s.deleteMachineToken)
		}

		// ==================== USER MANAGEMENT (ADMIN ONLY) ====================
		users := v1.Group("/users")
		users.Use(s.authService.AuthMiddleware())
		users.Use(auth.RequirePermission(auth.PermAdmin))
		{
			users.POST("", s.createUser)
			users.GET("", s.listUsers)
			users.PATCH("/:id", s.updateUser)
			users.DELETE("/:id", s.deleteUser)
		}

		// ==================== SYSTEM (OPERATOR+) ====================
		system := v1.Group("/system")
		system.Use(s.authService.AuthMiddleware())
		system.Use(auth.RequirePermission(auth.PermOperator))
		{
			system.GET("/status", s.getSystemStatus)
			system.POST("/update", s.triggerUpdate) // Maybe restrict to Admin
			system.POST("/shutdown", s.shutdown)    // Maybe restrict to Admin
		}

		// ==================== DEVICES ====================
		devices := v1.Group("/devices")
		devices.Use(s.authService.AuthMiddleware())
		{
			// Read operations: Operator+
			devices.GET("", auth.RequirePermission(auth.PermOperator), s.listDevices)
			devices.GET("/:id", auth.RequirePermission(auth.PermOperator), s.getDevice)
			devices.POST("/:id/read", auth.RequirePermission(auth.PermOperator), s.readRegister)

			// Write operations: Technician+
			devices.POST("", auth.RequirePermission(auth.PermAdmin), s.createDevice)
			devices.DELETE("/:id", auth.RequirePermission(auth.PermAdmin), s.deleteDevice)
			devices.POST("/:id/write", auth.RequirePermission(auth.PermTechnician), s.writeRegister)
		}

		// ==================== WORKFLOWS ====================
		workflows := v1.Group("/workflows")
		workflows.Use(s.authService.AuthMiddleware())
		{
			// Read & Execute: Operator+
			workflows.GET("", auth.RequirePermission(auth.PermOperator), s.listWorkflows)
			workflows.GET("/:id", auth.RequirePermission(auth.PermOperator), s.getWorkflow)
			workflows.POST("/:id/execute", auth.RequirePermission(auth.PermOperator), s.executeWorkflow)

			// Modify: Admin only
			workflows.POST("", auth.RequirePermission(auth.PermAdmin), s.createWorkflow)
			workflows.PUT("/:id", auth.RequirePermission(auth.PermAdmin), s.updateWorkflow)
			workflows.DELETE("/:id", auth.RequirePermission(auth.PermAdmin), s.deleteWorkflow)
			workflows.POST("/:id/activate", auth.RequirePermission(auth.PermAdmin), s.activateWorkflow)
		}

		// ==================== EXECUTIONS (OPERATOR+) ====================
		executions := v1.Group("/executions")
		executions.Use(s.authService.AuthMiddleware())
		executions.Use(auth.RequirePermission(auth.PermOperator))
		{
			executions.GET("/:id", s.getExecutionStatus)
			executions.GET("/:id/steps", s.getExecutionSteps)
			executions.POST("/:id/cancel", s.cancelExecution)
		}

		// ==================== MODULES (OPERATOR+) ====================
		modules := v1.Group("/modules")
		modules.Use(s.authService.AuthMiddleware())
		modules.Use(auth.RequirePermission(auth.PermOperator))
		{
			modules.GET("", s.listModules)
			modules.GET("/:vendor", s.getVendorModules)
			modules.GET("/:vendor/:model", s.getModule)
		}

		// ==================== MACHINE CONTROL (OPERATOR+) ====================
		machine := v1.Group("/machine")
		machine.Use(s.authService.AuthMiddleware())
		machine.Use(auth.RequirePermission(auth.PermOperator))
		{
			machine.GET("/status", s.getMachineStatus)
			machine.POST("/command", s.executeMachineCommand)
			machine.POST("/configure", auth.RequirePermission(auth.PermAdmin), s.configureMachineWorkflows)
		}

		// ==================== WEBSOCKET (PUBLIC - Auth via first message) ====================
		ws := v1.Group("/ws")
		{
			ws.GET("/live", s.wsLiveConnection)
			ws.GET("/status", auth.RequirePermission(auth.PermOperator), s.wsStatus)
		}
	}
}

// WebSocket handlers
func (s *Server) wsLiveConnection(c *gin.Context) {
	websocket.ServeWs(s.wsHub, c.Writer, c.Request)
}

func (s *Server) wsStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"connected_clients": s.wsHub.GetClientCount(),
	})
}

// Health check (public)
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	})
}

// Add missing execution handler
func (s *Server) cancelExecution(c *gin.Context) {
	executionID := c.Param("id")

	// Parse UUID
	execUUID, err := uuid.Parse(executionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution id"})
		return
	}

	// Get workflow engine from lifecycle manager
	engine := s.lm.WorkflowEngine()
	if engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "workflow engine not available"})
		return
	}

	if err := engine.CancelExecution(c.Request.Context(), execUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "execution cancelled"})
}
