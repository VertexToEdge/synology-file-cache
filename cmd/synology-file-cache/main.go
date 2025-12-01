package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/config"
	"github.com/vertextoedge/synology-file-cache/internal/httpapi"
	"github.com/vertextoedge/synology-file-cache/internal/logger"
	"github.com/vertextoedge/synology-file-cache/internal/store"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Init(cfg.Logging.Level, cfg.Logging.Format); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Log.Info("Starting synology-file-cache",
		"version", "0.1.0",
		"config", *configPath,
	)

	// Ensure cache root directory exists
	if err := os.MkdirAll(cfg.Cache.RootDir, 0755); err != nil {
		logger.Log.Fatalw("Failed to create cache directory", "error", err, "path", cfg.Cache.RootDir)
	}

	// Open database
	dbPath := filepath.Join(cfg.Cache.RootDir, "cache.db")
	db, err := store.Open(dbPath)
	if err != nil {
		logger.Log.Fatalw("Failed to open database", "error", err, "path", dbPath)
	}
	defer db.Close()

	// Create HTTP server
	httpServer := httpapi.NewServer(cfg.HTTP.BindAddr, db)

	// Start HTTP server in a goroutine
	go func() {
		if err := httpServer.Start(); err != nil {
			logger.Log.Fatalw("HTTP server failed", "error", err)
		}
	}()

	// TODO: Initialize and start syncer
	// syncer := syncer.New(cfg, db, synoClient)
	// go syncer.Start()

	// TODO: Initialize and start cacher
	// cacher := cacher.New(cfg, db, synoClient, fsManager)
	// go cacher.Start()

	// Wait for interrupt signal to gracefully shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	logger.Log.Info("Application started successfully")
	<-sigChan

	logger.Log.Info("Shutdown signal received, stopping services...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop HTTP server
	if err := httpServer.Stop(ctx); err != nil {
		logger.Log.Errorw("Failed to stop HTTP server gracefully", "error", err)
	}

	// TODO: Stop syncer and cacher
	// syncer.Stop(ctx)
	// cacher.Stop(ctx)

	logger.Log.Info("Application stopped successfully")
}