package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/config"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/server"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.New(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	log.Info("Starting Global Sync Service",
		logger.String("version", cfg.Version),
		logger.String("environment", cfg.Environment),
		logger.String("region", cfg.Region),
	)

	// Initialize server (connects to DB, Redis)
	srv, err := server.New(cfg, log)
	if err != nil {
		log.Fatal("Failed to initialize server", logger.Error(err))
	}

	// Start HTTP server in a goroutine
	go func() {
		addr := fmt.Sprintf(":%d", cfg.HTTPPort)
		log.Info("HTTP server starting",
			logger.String("addr", addr),
			logger.Int("port", cfg.HTTPPort),
		)
		if err := srv.Listen(addr); err != nil && err != http.ErrServerClosed {
			log.Fatal("HTTP server failed",
				logger.Error(err),
				logger.String("addr", addr),
				logger.Int("port", cfg.HTTPPort),
			)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown", logger.Error(err))
		os.Exit(1)
	}

	log.Info("Server exited properly")
}
