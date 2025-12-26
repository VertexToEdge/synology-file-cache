package cacher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// Config contains cacher configuration
type Config struct {
	MaxSizeBytes           int64
	MaxDiskUsagePercent    float64
	ConcurrentDownloads    int
	EvictionInterval       time.Duration
	StaleTaskTimeout       time.Duration
	ProgressUpdateInterval time.Duration
}

// DefaultConfig returns default cacher configuration
func DefaultConfig() *Config {
	return &Config{
		MaxSizeBytes:           50 * 1024 * 1024 * 1024, // 50GB
		MaxDiskUsagePercent:    50,
		ConcurrentDownloads:    3,
		EvictionInterval:       30 * time.Second,
		StaleTaskTimeout:       30 * time.Minute,
		ProgressUpdateInterval: 10 * time.Second,
	}
}

// Cacher handles file caching using a task queue
type Cacher struct {
	config     *Config
	drive      port.DriveClient
	files      port.FileRepository
	tasks      port.DownloadTaskRepository
	fs         port.FileSystem
	logger     *zap.Logger
	downloader *Downloader
	evictor    *Evictor

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// New creates a new Cacher
func New(
	cfg *Config,
	drive port.DriveClient,
	files port.FileRepository,
	tasks port.DownloadTaskRepository,
	fs port.FileSystem,
	logger *zap.Logger,
) *Cacher {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.ConcurrentDownloads == 0 {
		cfg.ConcurrentDownloads = 3
	}
	if cfg.EvictionInterval == 0 {
		cfg.EvictionInterval = 30 * time.Second
	}
	if cfg.StaleTaskTimeout == 0 {
		cfg.StaleTaskTimeout = 30 * time.Minute
	}
	if cfg.ProgressUpdateInterval == 0 {
		cfg.ProgressUpdateInterval = 10 * time.Second
	}

	c := &Cacher{
		config: cfg,
		drive:  drive,
		files:  files,
		tasks:  tasks,
		fs:     fs,
		logger: logger,
	}

	c.downloader = NewDownloader(drive, files, tasks, fs, logger, cfg.MaxSizeBytes, cfg.ProgressUpdateInterval)
	c.evictor = NewEvictor(files, tasks, fs, logger, cfg.EvictionInterval)

	return c
}

// Start starts the caching workers
func (c *Cacher) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("cacher already running")
	}
	c.running = true
	ctx, c.cancel = context.WithCancel(ctx)
	c.mu.Unlock()

	c.logger.Info("cacher started",
		zap.Int("workers", c.config.ConcurrentDownloads))

	// Release any stale tasks from previous run
	released, err := c.tasks.ReleaseStaleInProgressTasks(0) // Release all in-progress tasks on startup
	if err != nil {
		c.logger.Warn("failed to release stale tasks on startup", zap.Error(err))
	} else if released > 0 {
		c.logger.Info("released stale tasks from previous run", zap.Int("count", released))
	}

	// Start worker pool
	for i := 0; i < c.config.ConcurrentDownloads; i++ {
		c.wg.Add(1)
		go c.worker(ctx, i)
	}

	// Start maintenance loop
	c.wg.Add(1)
	go c.maintenanceLoop(ctx)

	<-ctx.Done()
	c.wg.Wait()
	c.logger.Info("cacher stopped")
	return nil
}

// Stop stops the cacher
func (c *Cacher) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
	c.running = false
}

// worker processes tasks from the queue
func (c *Cacher) worker(ctx context.Context, workerID int) {
	defer c.wg.Done()

	workerName := fmt.Sprintf("worker-%d", workerID)
	c.logger.Debug("cacher worker started", zap.String("worker", workerName))

	for {
		select {
		case <-ctx.Done():
			c.logger.Debug("cacher worker stopped", zap.String("worker", workerName))
			return
		default:
		}

		// Claim next task
		task, err := c.tasks.ClaimNextTask(workerName)
		if err != nil {
			c.logger.Error("failed to claim task",
				zap.String("worker", workerName),
				zap.Error(err))
			time.Sleep(5 * time.Second)
			continue
		}

		if task == nil {
			// No tasks available, wait before polling again
			time.Sleep(time.Second)
			continue
		}

		c.logger.Info("claimed download task",
			zap.String("worker", workerName),
			zap.String("path", task.SynoPath),
			zap.Int("priority", task.Priority),
			zap.Int64("bytes_downloaded", task.BytesDownloaded))

		// Process the task
		if err := c.processTask(ctx, task, workerName); err != nil {
			c.logger.Error("task failed",
				zap.String("worker", workerName),
				zap.String("path", task.SynoPath),
				zap.Int("retry_count", task.RetryCount),
				zap.Error(err))

			// Determine if we should retry
			canRetry := task.RetryCount < task.MaxRetries
			if err := c.tasks.FailTask(task.ID, err.Error(), canRetry); err != nil {
				c.logger.Error("failed to mark task as failed",
					zap.Int64("task_id", task.ID),
					zap.Error(err))
			}
		} else {
			if err := c.tasks.CompleteTask(task.ID); err != nil {
				c.logger.Error("failed to complete task",
					zap.Int64("task_id", task.ID),
					zap.Error(err))
			}
		}
	}
}

// processTask handles a single download task
func (c *Cacher) processTask(ctx context.Context, task *domain.DownloadTask, workerName string) error {
	// Get the file record
	file, err := c.files.GetByID(task.FileID)
	if err != nil {
		return fmt.Errorf("failed to get file: %w", err)
	}
	if file == nil {
		return fmt.Errorf("file not found: %d", task.FileID)
	}

	// Check if file is already cached
	if file.Cached {
		c.logger.Debug("file already cached, skipping",
			zap.String("path", task.SynoPath))
		return nil
	}

	// Check space
	hasSpace, availableBytes, err := c.checkSpaceForFile(task.Size)
	if err != nil {
		return fmt.Errorf("space check failed: %w", err)
	}

	if !hasSpace {
		c.logger.Debug("not enough space, attempting eviction",
			zap.String("worker", workerName),
			zap.Int64("needed", task.Size),
			zap.Int64("available", availableBytes))

		if err := c.evictor.TryEvict(ctx, task.Size, c.config.MaxSizeBytes, c.config.MaxDiskUsagePercent); err != nil {
			c.logger.Warn("eviction failed or rate-limited",
				zap.String("worker", workerName),
				zap.String("path", task.SynoPath),
				zap.Error(err))
			return domain.ErrInsufficientSpace
		}

		// Re-check space after eviction
		hasSpace, _, err = c.checkSpaceForFile(task.Size)
		if err != nil || !hasSpace {
			return domain.ErrInsufficientSpace
		}
	}

	// Download with task
	return c.downloader.DownloadWithTask(ctx, file, task)
}

// checkSpaceForFile checks if there's enough space for a file
func (c *Cacher) checkSpaceForFile(fileSize int64) (bool, int64, error) {
	// Check cache size limit
	cacheSize, err := c.fs.GetCacheSize()
	if err != nil {
		return false, 0, err
	}

	availableBySize := c.config.MaxSizeBytes - cacheSize
	if cacheSize+fileSize > c.config.MaxSizeBytes {
		return false, availableBySize, nil
	}

	// Check disk usage limit
	usage, err := c.fs.GetDiskUsage()
	if err != nil {
		return false, 0, err
	}

	if usage.UsedPct >= c.config.MaxDiskUsagePercent {
		return false, availableBySize, nil
	}

	// Check if adding this file would exceed disk limit
	newUsedPct := float64(usage.Used+uint64(fileSize)) / float64(usage.Total) * 100
	if newUsedPct >= c.config.MaxDiskUsagePercent {
		return false, availableBySize, nil
	}

	return true, availableBySize, nil
}

// maintenanceLoop handles periodic maintenance tasks
func (c *Cacher) maintenanceLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	// Cleanup ticker for failed tasks
	cleanupTicker := time.NewTicker(time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Release stale in-progress tasks (worker died)
			released, err := c.tasks.ReleaseStaleInProgressTasks(c.config.StaleTaskTimeout)
			if err != nil {
				c.logger.Error("failed to release stale tasks", zap.Error(err))
			} else if released > 0 {
				c.logger.Info("released stale tasks", zap.Int("count", released))
			}
		case <-cleanupTicker.C:
			// Clear old failed tasks
			cleared, err := c.tasks.CleanupOldFailedTasks(24 * time.Hour)
			if err != nil {
				c.logger.Error("failed to cleanup failed tasks", zap.Error(err))
			} else if cleared > 0 {
				c.logger.Info("cleaned up old failed tasks", zap.Int("count", cleared))
			}

			// Clean old temp files from filesystem
			fileCount, err := c.fs.CleanOldTempFiles(24 * time.Hour)
			if err != nil {
				c.logger.Error("failed to cleanup old temp files", zap.Error(err))
			} else if fileCount > 0 {
				c.logger.Info("cleaned up old temp files from filesystem", zap.Int("count", fileCount))
			}
		}
	}
}

// GetStats returns caching statistics
func (c *Cacher) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	cacheSize, err := c.fs.GetCacheSize()
	if err != nil {
		return nil, err
	}
	stats["cache_size_bytes"] = cacheSize
	stats["max_size_bytes"] = c.config.MaxSizeBytes

	usage, err := c.fs.GetDiskUsage()
	if err != nil {
		return nil, err
	}
	stats["disk_total_bytes"] = usage.Total
	stats["disk_used_bytes"] = usage.Used
	stats["disk_used_percent"] = usage.UsedPct
	stats["max_disk_percent"] = c.config.MaxDiskUsagePercent

	// Add queue stats
	queueStats, err := c.tasks.GetQueueStats()
	if err != nil {
		c.logger.Warn("failed to get queue stats", zap.Error(err))
	} else {
		stats["queue_pending"] = queueStats.PendingCount
		stats["queue_in_progress"] = queueStats.InProgressCount
		stats["queue_failed"] = queueStats.FailedCount
		stats["queue_total_bytes"] = queueStats.TotalBytesQueued
	}

	return stats, nil
}
