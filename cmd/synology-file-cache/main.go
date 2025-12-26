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

	"github.com/vertextoedge/synology-file-cache/internal/adapter/filesystem"
	"github.com/vertextoedge/synology-file-cache/internal/adapter/sqlite"
	"github.com/vertextoedge/synology-file-cache/internal/adapter/synology"
	"github.com/vertextoedge/synology-file-cache/internal/config"
	"github.com/vertextoedge/synology-file-cache/internal/logger"
	"github.com/vertextoedge/synology-file-cache/internal/service/cacher"
	"github.com/vertextoedge/synology-file-cache/internal/service/maintenance"
	"github.com/vertextoedge/synology-file-cache/internal/service/server"
	"github.com/vertextoedge/synology-file-cache/internal/service/syncer"
	"go.uber.org/zap"
)

const version = "0.2.0"

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

	zapLogger := logger.GetZapLogger()
	zapLogger.Info("starting synology-file-cache",
		zap.String("version", version),
		zap.String("config", *configPath),
	)

	// Initialize filesystem manager
	fsManager, err := filesystem.NewManagerWithBufferSize(cfg.Cache.RootDir, cfg.Cache.GetBufferSize())
	if err != nil {
		zapLogger.Fatal("failed to create filesystem manager", zap.Error(err))
	}

	// Open database
	dbPath := cfg.Database.Path
	if dbPath == "" {
		dbPath = filepath.Join(cfg.Cache.RootDir, "cache.db")
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		zapLogger.Fatal("failed to open database", zap.Error(err), zap.String("path", dbPath))
	}
	defer store.Close()

	// Create Synology API client
	synoClient := synology.NewClientWithConfig(
		cfg.Synology.BaseURL,
		cfg.Synology.Username,
		cfg.Synology.Password,
		cfg.Synology.SkipTLSVerify,
		&synology.ClientConfig{
			BufferSizeMB: cfg.Cache.BufferSizeMB,
		},
	)

	// Login to Synology
	if err := synoClient.Login(); err != nil {
		zapLogger.Fatal("failed to login to Synology", zap.Error(err))
	}
	zapLogger.Info("connected to Synology NAS", zap.String("url", cfg.Synology.BaseURL))

	// Create Drive client
	driveClient := synology.NewDriveClient(synoClient)

	// Create syncer
	syncerCfg := &syncer.Config{
		FullScanInterval:    cfg.Sync.GetFullScanInterval(),
		IncrementalInterval: cfg.Sync.GetIncrementalInterval(),
		RecentModifiedDays:  cfg.Cache.RecentModifiedDays,
		RecentAccessedDays:  cfg.Cache.RecentAccessedDays,
		ExcludeLabels:       cfg.Sync.ExcludeLabels,
		PageSize:            cfg.Sync.GetPageSize(),
		MaxDownloadRetries:  cfg.Cache.GetMaxDownloadRetries(),
		MaxCacheSize:        int64(cfg.Cache.MaxSizeGB) * 1024 * 1024 * 1024,
	}
	syncerService := syncer.New(syncerCfg, driveClient, store, store, store, zapLogger)

	// Create cacher
	cacherCfg := &cacher.Config{
		MaxSizeBytes:           int64(cfg.Cache.MaxSizeGB) * 1024 * 1024 * 1024,
		MaxDiskUsagePercent:    float64(cfg.Cache.MaxDiskUsagePercent),
		EvictionInterval:       cfg.Cache.GetEvictionInterval(),
		ConcurrentDownloads:    cfg.Cache.ConcurrentDownloads,
		StaleTaskTimeout:       cfg.Cache.GetStaleTaskTimeout(),
		ProgressUpdateInterval: cfg.Cache.GetProgressUpdateInterval(),
		WorkerPollInterval:     cfg.Cache.GetWorkerPollInterval(),
		WorkerErrorBackoff:     cfg.Cache.GetWorkerErrorBackoff(),
		EvictionBatchSize:      cfg.Cache.GetEvictionBatchSize(),
		MaxDownloadRetries:     cfg.Cache.GetMaxDownloadRetries(),
	}
	cacherService := cacher.New(cacherCfg, driveClient, store, store, fsManager, zapLogger)

	// Create maintenance service
	maintenanceCfg := &maintenance.Config{
		StaleTaskCheckInterval: time.Minute,
		StaleTaskTimeout:       cfg.Cache.GetStaleTaskTimeout(),
		CleanupInterval:        time.Hour,
		FailedTaskMaxAge:       24 * time.Hour,
		TempFileMaxAge:         24 * time.Hour,
	}
	maintenanceService := maintenance.New(maintenanceCfg, store, fsManager, zapLogger)

	// Create HTTP server
	serverCfg := &server.Config{
		BindAddr:           cfg.HTTP.BindAddr,
		AdminUsername:      cfg.Synology.Username,
		AdminPassword:      cfg.Synology.Password,
		EnableAdminBrowser: cfg.HTTP.EnableAdminBrowser,
		CacheRootDir:       cfg.Cache.RootDir,
		ReadTimeout:        cfg.HTTP.GetReadTimeout(),
		WriteTimeout:       cfg.HTTP.GetWriteTimeout(),
		IdleTimeout:        cfg.HTTP.GetIdleTimeout(),
	}
	httpServer := server.New(serverCfg, store, zapLogger)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start HTTP server
	go func() {
		if err := httpServer.Start(); err != nil {
			zapLogger.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// Start syncer
	go func() {
		if err := syncerService.Start(ctx); err != nil && err != context.Canceled {
			zapLogger.Error("syncer stopped with error", zap.Error(err))
		}
	}()

	// Start cacher
	go func() {
		if err := cacherService.Start(ctx); err != nil && err != context.Canceled {
			zapLogger.Error("cacher stopped with error", zap.Error(err))
		}
	}()

	// Start maintenance service
	go func() {
		if err := maintenanceService.Start(ctx); err != nil && err != context.Canceled {
			zapLogger.Error("maintenance service stopped with error", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	zapLogger.Info("application started successfully",
		zap.String("http_addr", cfg.HTTP.BindAddr),
		zap.String("cache_dir", cfg.Cache.RootDir),
	)
	<-sigChan

	zapLogger.Info("shutdown signal received, stopping services...")

	// Cancel context to stop syncer and cacher
	cancel()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop syncer, cacher, and maintenance services
	syncerService.Stop()
	cacherService.Stop()
	maintenanceService.Stop()

	// Stop HTTP server
	if err := httpServer.Stop(shutdownCtx); err != nil {
		zapLogger.Error("failed to stop HTTP server gracefully", zap.Error(err))
	}

	// Logout from Synology
	if err := synoClient.Logout(); err != nil {
		zapLogger.Error("failed to logout from Synology", zap.Error(err))
	}

	zapLogger.Info("application stopped successfully")
}
