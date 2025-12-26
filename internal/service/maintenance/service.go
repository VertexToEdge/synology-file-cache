package maintenance

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// Config contains maintenance service configuration
type Config struct {
	// StaleTaskCheckInterval is how often to check for stale tasks
	StaleTaskCheckInterval time.Duration

	// StaleTaskTimeout is when a task is considered stale
	StaleTaskTimeout time.Duration

	// CleanupInterval is how often to run cleanup tasks
	CleanupInterval time.Duration

	// FailedTaskMaxAge is the maximum age of failed tasks before cleanup
	FailedTaskMaxAge time.Duration

	// TempFileMaxAge is the maximum age of temp files before cleanup
	TempFileMaxAge time.Duration
}

// DefaultConfig returns default maintenance configuration
func DefaultConfig() *Config {
	return &Config{
		StaleTaskCheckInterval: time.Minute,
		StaleTaskTimeout:       30 * time.Minute,
		CleanupInterval:        time.Hour,
		FailedTaskMaxAge:       24 * time.Hour,
		TempFileMaxAge:         24 * time.Hour,
	}
}

// Service handles periodic maintenance tasks
type Service struct {
	config *Config
	tasks  port.DownloadTaskRepository
	fs     port.FileSystem
	logger *zap.Logger

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// New creates a new maintenance Service
func New(cfg *Config, tasks port.DownloadTaskRepository, fs port.FileSystem, logger *zap.Logger) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.StaleTaskCheckInterval == 0 {
		cfg.StaleTaskCheckInterval = time.Minute
	}
	if cfg.StaleTaskTimeout == 0 {
		cfg.StaleTaskTimeout = 30 * time.Minute
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = time.Hour
	}
	if cfg.FailedTaskMaxAge == 0 {
		cfg.FailedTaskMaxAge = 24 * time.Hour
	}
	if cfg.TempFileMaxAge == 0 {
		cfg.TempFileMaxAge = 24 * time.Hour
	}

	return &Service{
		config: cfg,
		tasks:  tasks,
		fs:     fs,
		logger: logger,
	}
}

// Start starts the maintenance service
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("maintenance service already running")
	}
	s.running = true
	ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	s.logger.Info("maintenance service started",
		zap.Duration("stale_check_interval", s.config.StaleTaskCheckInterval),
		zap.Duration("cleanup_interval", s.config.CleanupInterval))

	s.wg.Add(1)
	go s.maintenanceLoop(ctx)

	<-ctx.Done()
	s.wg.Wait()
	s.logger.Info("maintenance service stopped")
	return nil
}

// Stop stops the maintenance service
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
}

// maintenanceLoop handles periodic maintenance tasks
func (s *Service) maintenanceLoop(ctx context.Context) {
	defer s.wg.Done()

	staleTaskTicker := time.NewTicker(s.config.StaleTaskCheckInterval)
	defer staleTaskTicker.Stop()

	cleanupTicker := time.NewTicker(s.config.CleanupInterval)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-staleTaskTicker.C:
			s.releaseStaleTask()
		case <-cleanupTicker.C:
			s.cleanupFailedTasks()
			s.cleanupTempFiles()
		}
	}
}

// releaseStaleTask releases tasks that have been in progress for too long
func (s *Service) releaseStaleTask() {
	released, err := s.tasks.ReleaseStaleInProgressTasks(s.config.StaleTaskTimeout)
	if err != nil {
		s.logger.Error("failed to release stale tasks", zap.Error(err))
	} else if released > 0 {
		s.logger.Info("released stale tasks", zap.Int("count", released))
	}
}

// cleanupFailedTasks removes old failed tasks
func (s *Service) cleanupFailedTasks() {
	cleared, err := s.tasks.CleanupOldFailedTasks(s.config.FailedTaskMaxAge)
	if err != nil {
		s.logger.Error("failed to cleanup failed tasks", zap.Error(err))
	} else if cleared > 0 {
		s.logger.Info("cleaned up old failed tasks", zap.Int("count", cleared))
	}
}

// cleanupTempFiles removes old temporary files from the filesystem
func (s *Service) cleanupTempFiles() {
	fileCount, err := s.fs.CleanOldTempFiles(s.config.TempFileMaxAge)
	if err != nil {
		s.logger.Error("failed to cleanup old temp files", zap.Error(err))
	} else if fileCount > 0 {
		s.logger.Info("cleaned up old temp files from filesystem", zap.Int("count", fileCount))
	}
}
