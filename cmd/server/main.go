package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/auth"
	"github.com/KevinKickass/OpenMachineCore/internal/config"
	"github.com/KevinKickass/OpenMachineCore/internal/storage"
	"github.com/KevinKickass/OpenMachineCore/internal/system"
	"go.uber.org/zap"
)

var (
	generateToken = flag.String("generate-machine-token", "", "Generate a new machine token with the given name")
	createAdmin   = flag.Bool("create-admin", false, "Create default admin user (username: admin, password: admin123)")
	configPath    = flag.String("config", "configs/config.yaml", "Path to configuration file")
)

func main() {
	flag.Parse()

	// Logger initialisieren
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Config laden (verwendet Viper - unterstützt YAML + ENV)
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Security Check: JWT Secret
	if !cfg.Auth.IsProductionReady() {
		logger.Warn("WARNING: Using default or insecure JWT secret!",
			zap.String("recommendation", "Set environment variable JWT_SECRET with at least 32 characters"))
	}

	// Database Connection
	pgClient, err := storage.NewPostgresClient(cfg.Database)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer pgClient.Close()

	// Auth Service (verwendet Config inkl. JWT Secret aus ENV)
	authService := auth.NewAuthService(pgClient, cfg.Auth)

	ctx := context.Background()

	// ==================== CLI COMMANDS ====================

	// Generate Machine Token
	if *generateToken != "" {
		token, machineToken, err := authService.CreateMachineToken(
			ctx,
			*generateToken,
			[]string{"operator"},
			nil,
			map[string]interface{}{
				"created_via": "cli",
			},
		)
		if err != nil {
			logger.Fatal("Failed to generate machine token", zap.Error(err))
		}

		fmt.Println("\nMachine Token Generated Successfully!")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("Name:        %s\n", machineToken.Name)
		fmt.Printf("ID:          %s\n", machineToken.ID)
		fmt.Printf("Permissions: %v\n", machineToken.Permissions)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("Token: %s\n", token)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("\nIMPORTANT: Save this token securely!")
		fmt.Println("   It will NOT be displayed again.")
		fmt.Println("   Use it in your HMI/Configurator:")
		fmt.Printf("   export OMC_API_KEY=%s\n\n", token)

		os.Exit(0)
	}

	// Create Admin User
	if *createAdmin {
		user, err := authService.CreateUser(ctx, "admin", "admin123", "admin")
		if err != nil {
			logger.Fatal("Failed to create admin user", zap.Error(err))
		}

		fmt.Println("\nAdmin User Created Successfully!")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("Username: %s\n", user.Username)
		fmt.Printf("Password: admin123\n")
		fmt.Printf("Role:     %s\n", user.Role)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("\nCHANGE THE PASSWORD IMMEDIATELY IN PRODUCTION!")

		os.Exit(0)
	}

	// ==================== NORMAL SERVER START ====================

	logger.Info("Starting OpenMachineCore",
		zap.String("version", "0.1.0"),
		zap.Int("http_port", cfg.Server.HTTPPort),
		zap.Int("grpc_port", cfg.Server.GRPCPort))

	// System Lifecycle Manager MIT authService
	// KORRIGIERT: Richtige Parameter-Reihenfolge
	lifecycleManager := system.NewLifecycleManager(pgClient, cfg, logger, authService)

	// Start system - direkt ohne Initialize()
	if err := lifecycleManager.Start(); err != nil {
		logger.Fatal("Failed to start system", zap.Error(err))
	}

	logger.Info("OpenMachineCore started successfully")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down OpenMachineCore...")

	// KORRIGIERT: Shutdown mit Context
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := lifecycleManager.Shutdown(shutdownCtx); err != nil {
		logger.Error("System shutdown error", zap.Error(err))
	}

	logger.Info("OpenMachineCore stopped")
}
