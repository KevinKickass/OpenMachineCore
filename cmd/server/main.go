package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/KevinKickass/OpenMachineCore/internal/config"
	"github.com/KevinKickass/OpenMachineCore/internal/storage"
	"github.com/KevinKickass/OpenMachineCore/internal/system"
	"go.uber.org/zap"
)

func main() {
	// Logger initialisieren
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	// Config laden
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	logger.Info("Config loaded successfully")

	// PostgreSQL verbinden
	db, err := storage.NewPostgresClient(cfg.Database)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	logger.Info("Database connected successfully")

	// Lifecycle Manager
	lifecycle := system.NewLifecycleManager(db, cfg, logger)

	// System starten
	if err := lifecycle.Start(); err != nil {
		logger.Fatal("Failed to start system", zap.Error(err))
	}

	logger.Info("OpenMachineCore started successfully")

	// Graceful Shutdown auf Signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	logger.Info("Shutdown signal received")

	ctx := context.Background()
	if err := lifecycle.Shutdown(ctx); err != nil {
		logger.Error("Shutdown failed", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("OpenMachineCore stopped successfully")
}
