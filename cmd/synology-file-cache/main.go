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

	"github.com/vertextoedge/synology-file-cache/internal/cacher"
	"github.com/vertextoedge/synology-file-cache/internal/config"
	"github.com/vertextoedge/synology-file-cache/internal/fs"
	"github.com/vertextoedge/synology-file-cache/internal/httpapi"
	"github.com/vertextoedge/synology-file-cache/internal/logger"
	"github.com/vertextoedge/synology-file-cache/internal/store"
	"github.com/vertextoedge/synology-file-cache/internal/syncer"
	"github.com/vertextoedge/synology-file-cache/internal/synoapi"
	"go.uber.org/zap"
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

	// Initialize filesystem manager
	fsManager, err := fs.NewManager(cfg.Cache.RootDir)
	if err != nil {
		logger.Log.Fatalw("Failed to create filesystem manager", "error", err)
	}

	// Open database
	dbPath := filepath.Join(cfg.Cache.RootDir, "cache.db")
	db, err := store.Open(dbPath)
	if err != nil {
		logger.Log.Fatalw("Failed to open database", "error", err, "path", dbPath)
	}
	defer db.Close()

	// Create Synology API client
	synoClient := synoapi.NewClient(
		cfg.Synology.BaseURL,
		cfg.Synology.Username,
		cfg.Synology.Password,
		cfg.Synology.SkipTLSVerify,
	)

	// Login to Synology
	if err := synoClient.Login(); err != nil {
		logger.Log.Fatalw("Failed to login to Synology", "error", err)
	}
	logger.Log.Info("Connected to Synology NAS", "url", cfg.Synology.BaseURL)

	// Get zap logger for components
	zapLogger := logger.GetZapLogger()

	// Create syncer (using Drive API)
	syncerCfg := &syncer.Config{
		FullScanInterval:    cfg.Sync.GetFullScanInterval(),
		IncrementalInterval: cfg.Sync.GetIncrementalInterval(),
		RecentModifiedDays:  cfg.Cache.RecentModifiedDays,
		RecentAccessedDays:  cfg.Cache.RecentAccessedDays,
		ExcludeLabels:       cfg.Sync.ExcludeLabels,
	}
	driveSyncer := syncer.NewDriveSyncer(syncerCfg, synoClient, db, zapLogger)

	// Create cacher
	cacherCfg := &cacher.Config{
		MaxSizeBytes:        int64(cfg.Cache.MaxSizeGB) * 1024 * 1024 * 1024,
		MaxDiskUsagePercent: float64(cfg.Cache.MaxDiskUsagePercent),
		PrefetchInterval:    cfg.Sync.GetPrefetchInterval(),
		BatchSize:           10,
		ConcurrentDownloads: cfg.Cache.ConcurrentDownloads,
	}
	cacherInstance := cacher.New(cacherCfg, synoClient, db, fsManager, zapLogger)

	// Create HTTP server
	httpServer := httpapi.NewServer(
		cfg.HTTP.BindAddr,
		db,
		cfg.Synology.Username,
		cfg.Synology.Password,
		cfg.HTTP.EnableAdminBrowser,
		cfg.Cache.RootDir,
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start HTTP server
	go func() {
		if err := httpServer.Start(); err != nil {
			logger.Log.Fatalw("HTTP server failed", "error", err)
		}
	}()

	// Start syncer
	go func() {
		if err := driveSyncer.Start(ctx); err != nil && err != context.Canceled {
			logger.Log.Errorw("Syncer stopped with error", "error", err)
		}
	}()

	// Start cacher
	go func() {
		if err := cacherInstance.Start(ctx); err != nil && err != context.Canceled {
			logger.Log.Errorw("Cacher stopped with error", "error", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	logger.Log.Info("Application started successfully",
		zap.String("http_addr", cfg.HTTP.BindAddr),
		zap.String("cache_dir", cfg.Cache.RootDir),
	)
	<-sigChan

	logger.Log.Info("Shutdown signal received, stopping services...")

	// Cancel context to stop syncer and cacher
	cancel()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop syncer and cacher
	driveSyncer.Stop()
	cacherInstance.Stop()

	// Stop HTTP server
	if err := httpServer.Stop(shutdownCtx); err != nil {
		logger.Log.Errorw("Failed to stop HTTP server gracefully", "error", err)
	}

	// Logout from Synology
	if err := synoClient.Logout(); err != nil {
		logger.Log.Errorw("Failed to logout from Synology", "error", err)
	}

	logger.Log.Info("Application stopped successfully")
}
