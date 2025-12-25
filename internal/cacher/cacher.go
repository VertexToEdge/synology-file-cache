package cacher

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/fs"
	"github.com/vertextoedge/synology-file-cache/internal/store"
	"github.com/vertextoedge/synology-file-cache/internal/synoapi"
	"go.uber.org/zap"
)

// Config contains cacher configuration
type Config struct {
	MaxSizeBytes        int64   // Maximum cache size in bytes
	MaxDiskUsagePercent float64 // Maximum disk usage percentage
	PrefetchInterval    time.Duration
	BatchSize           int // Number of files to process per batch
	ConcurrentDownloads int // Number of parallel downloads
}

// Cacher handles file caching and eviction
type Cacher struct {
	config *Config
	client *synoapi.Client
	store  *store.Store
	fs     *fs.Manager
	logger *zap.Logger

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc

	// Eviction rate limiting
	lastEviction     time.Time
	evictionInterval time.Duration // Minimum interval between evictions
}

// New creates a new Cacher
func New(cfg *Config, client *synoapi.Client, st *store.Store, fsm *fs.Manager, logger *zap.Logger) *Cacher {
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 10
	}
	return &Cacher{
		config:           cfg,
		client:           client,
		store:            st,
		fs:               fsm,
		logger:           logger,
		evictionInterval: 30 * time.Second, // Minimum 30 seconds between evictions
	}
}

// Start starts the caching loop
func (c *Cacher) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("cacher already running")
	}
	c.running = true
	ctx, c.cancel = context.WithCancel(ctx)
	c.mu.Unlock()

	c.logger.Info("cacher started", zap.Duration("interval", c.config.PrefetchInterval))

	ticker := time.NewTicker(c.config.PrefetchInterval)
	defer ticker.Stop()

	// Run immediately on start
	c.runCacheLoop(ctx)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("cacher stopped")
			return nil
		case <-ticker.C:
			c.runCacheLoop(ctx)
		}
	}
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

// runCacheLoop performs one iteration of the cache loop
func (c *Cacher) runCacheLoop(ctx context.Context) {
	// Get files to cache (ordered by priority)
	files, err := c.store.GetFilesToCache(c.config.BatchSize)
	if err != nil {
		c.logger.Error("failed to get files to cache", zap.Error(err))
		return
	}

	if len(files) == 0 {
		c.logger.Debug("no files to cache")
		return
	}

	c.logger.Info("caching files",
		zap.Int("count", len(files)),
		zap.Int("workers", c.config.ConcurrentDownloads))

	// Create worker pool
	fileChan := make(chan *store.File, len(files))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < c.config.ConcurrentDownloads; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			c.cacheWorker(ctx, workerID, fileChan)
		}(i)
	}

	// Send files to workers
	for _, file := range files {
		select {
		case <-ctx.Done():
			close(fileChan)
			wg.Wait()
			return
		case fileChan <- file:
		}
	}

	// Close channel and wait for all workers to finish
	close(fileChan)
	wg.Wait()
}

// cacheWorker processes files from the file channel
func (c *Cacher) cacheWorker(ctx context.Context, workerID int, fileChan <-chan *store.File) {
	for {
		select {
		case <-ctx.Done():
			return
		case file, ok := <-fileChan:
			if !ok {
				return // Channel closed
			}

			// Check space before caching each file
			hasSpace, availableBytes, err := c.checkSpaceForFile(file.Size)
			if err != nil {
				c.logger.Error("failed to check disk space",
					zap.Int("worker", workerID),
					zap.Error(err))
				continue
			}

			if !hasSpace {
				// Try to evict files to make space (with rate limiting)
				if err := c.tryEvict(ctx, file.Size); err != nil {
					c.logger.Warn("eviction failed or rate-limited, skipping file",
						zap.Int("worker", workerID),
						zap.String("path", file.Path),
						zap.Int64("size", file.Size),
						zap.Int64("available", availableBytes),
						zap.Error(err))
					continue
				}

				// Re-check space after eviction
				hasSpace, _, err = c.checkSpaceForFile(file.Size)
				if err != nil || !hasSpace {
					c.logger.Warn("still not enough space after eviction, skipping file",
						zap.Int("worker", workerID),
						zap.String("path", file.Path),
						zap.Int64("size", file.Size))
					continue
				}
			}

			if err := c.cacheFile(ctx, file); err != nil {
				c.logger.Error("failed to cache file",
					zap.Int("worker", workerID),
					zap.String("path", file.Path),
					zap.Error(err))
				continue
			}
		}
	}
}

// cacheFile downloads and caches a single file
func (c *Cacher) cacheFile(ctx context.Context, file *store.File) error {
	c.logger.Debug("caching file",
		zap.String("path", file.Path),
		zap.Int("priority", file.Priority))

	// Download file from Synology Drive using path
	// Note: DriveDownloadFile can use either file_id or path
	body, _, _, err := c.client.DriveDownloadFile(0, file.Path)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer body.Close()

	// Write to cache
	cachePath, written, err := c.fs.WriteFile(file.Path, body)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	// Update database
	now := time.Now()
	file.Cached = true
	file.CachePath = sql.NullString{String: cachePath, Valid: true}
	file.LastAccessInCacheAt = &now
	file.Size = written

	if err := c.store.UpdateFile(file); err != nil {
		// Rollback: delete cached file
		c.fs.DeleteFile(cachePath)
		return fmt.Errorf("db update failed: %w", err)
	}

	c.logger.Info("file cached",
		zap.String("path", file.Path),
		zap.Int64("size", written),
		zap.Int("priority", file.Priority))

	return nil
}

// hasAvailableSpace checks if there's space for caching
func (c *Cacher) hasAvailableSpace() (bool, error) {
	hasSpace, _, err := c.checkSpaceForFile(0)
	return hasSpace, err
}

// checkSpaceForFile checks if there's enough space to cache a file of given size
// Returns: hasSpace, availableBytes, error
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

	// Also check if adding this file would exceed disk limit
	newUsedPct := float64(usage.Used+uint64(fileSize)) / float64(usage.Total) * 100
	if newUsedPct >= c.config.MaxDiskUsagePercent {
		return false, availableBySize, nil
	}

	return true, availableBySize, nil
}

// tryEvict attempts to evict files with rate limiting
// Returns error if rate-limited or eviction fails
func (c *Cacher) tryEvict(ctx context.Context, neededBytes int64) error {
	c.mu.Lock()
	timeSinceLastEviction := time.Since(c.lastEviction)
	if timeSinceLastEviction < c.evictionInterval {
		c.mu.Unlock()
		return fmt.Errorf("eviction rate-limited: next eviction in %v", c.evictionInterval-timeSinceLastEviction)
	}
	c.lastEviction = time.Now()
	c.mu.Unlock()

	c.logger.Info("starting eviction",
		zap.Int64("needed_bytes", neededBytes),
		zap.Duration("since_last", timeSinceLastEviction))

	return c.evictFilesUntilSpace(ctx, neededBytes)
}

// evictFilesUntilSpace evicts files until enough space is available
func (c *Cacher) evictFilesUntilSpace(ctx context.Context, neededBytes int64) error {
	evictedCount := 0
	evictedBytes := int64(0)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if we have enough space now
		hasSpace, _, err := c.checkSpaceForFile(neededBytes)
		if err != nil {
			return err
		}
		if hasSpace {
			c.logger.Info("eviction completed",
				zap.Int("evicted_count", evictedCount),
				zap.Int64("evicted_bytes", evictedBytes))
			return nil
		}

		// Get one candidate for eviction
		candidates, err := c.store.GetEvictionCandidates(1)
		if err != nil {
			return fmt.Errorf("failed to get eviction candidates: %w", err)
		}

		if len(candidates) == 0 {
			return fmt.Errorf("no eviction candidates available")
		}

		file := candidates[0]

		// Evict file
		if file.CachePath.Valid {
			fileSize := file.Size
			if err := c.fs.DeleteFile(file.CachePath.String); err != nil {
				c.logger.Error("failed to delete cached file",
					zap.String("path", file.CachePath.String),
					zap.Error(err))
				continue
			}
			evictedBytes += fileSize
		}

		// Update database
		file.Cached = false
		file.CachePath = sql.NullString{}
		file.LastAccessInCacheAt = nil

		if err := c.store.UpdateFile(file); err != nil {
			c.logger.Error("failed to update file after eviction",
				zap.String("path", file.Path),
				zap.Error(err))
			continue
		}

		evictedCount++
		c.logger.Debug("file evicted",
			zap.String("path", file.Path),
			zap.Int("priority", file.Priority),
			zap.Int64("size", file.Size))
	}
}

// evictFiles removes files to free up space
func (c *Cacher) evictFiles(ctx context.Context) error {
	// Get candidates for eviction (highest priority number first, then LRU)
	candidates, err := c.store.GetEvictionCandidates(c.config.BatchSize)
	if err != nil {
		return fmt.Errorf("failed to get eviction candidates: %w", err)
	}

	if len(candidates) == 0 {
		c.logger.Warn("no eviction candidates found")
		return nil
	}

	for _, file := range candidates {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if we have enough space now
		hasSpace, err := c.hasAvailableSpace()
		if err != nil {
			return err
		}
		if hasSpace {
			break
		}

		// Evict file
		if file.CachePath.Valid {
			if err := c.fs.DeleteFile(file.CachePath.String); err != nil {
				c.logger.Error("failed to delete cached file",
					zap.String("path", file.CachePath.String),
					zap.Error(err))
				continue
			}
		}

		// Update database
		file.Cached = false
		file.CachePath = sql.NullString{}
		file.LastAccessInCacheAt = nil

		if err := c.store.UpdateFile(file); err != nil {
			c.logger.Error("failed to update file after eviction",
				zap.String("path", file.Path),
				zap.Error(err))
			continue
		}

		c.logger.Info("file evicted",
			zap.String("path", file.Path),
			zap.Int("priority", file.Priority))
	}

	return nil
}

// CacheFileNow immediately caches a specific file (for on-demand caching)
func (c *Cacher) CacheFileNow(ctx context.Context, file *store.File) error {
	return c.cacheFile(ctx, file)
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

	return stats, nil
}
