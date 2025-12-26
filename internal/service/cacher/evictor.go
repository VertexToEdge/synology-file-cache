package cacher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// Evictor handles cache eviction
type Evictor struct {
	files  port.FileRepository
	tasks  port.DownloadTaskRepository
	fs     port.FileSystem
	logger *zap.Logger

	mu               sync.Mutex
	lastEviction     time.Time
	evictionInterval time.Duration
}

// NewEvictor creates a new Evictor
func NewEvictor(files port.FileRepository, tasks port.DownloadTaskRepository, fs port.FileSystem, logger *zap.Logger, evictionInterval time.Duration) *Evictor {
	return &Evictor{
		files:            files,
		tasks:            tasks,
		fs:               fs,
		logger:           logger,
		evictionInterval: evictionInterval,
	}
}

// TryEvict attempts to evict files with rate limiting
func (e *Evictor) TryEvict(ctx context.Context, neededBytes int64, maxCacheSize int64, maxDiskUsagePct float64) error {
	e.mu.Lock()
	timeSinceLastEviction := time.Since(e.lastEviction)
	if timeSinceLastEviction < e.evictionInterval {
		e.mu.Unlock()
		return fmt.Errorf("eviction rate-limited: next eviction in %v", e.evictionInterval-timeSinceLastEviction)
	}
	e.lastEviction = time.Now()
	e.mu.Unlock()

	e.logger.Info("starting eviction",
		zap.Int64("needed_bytes", neededBytes),
		zap.Duration("since_last", timeSinceLastEviction))

	return e.evictUntilSpace(ctx, neededBytes, maxCacheSize, maxDiskUsagePct)
}

// evictUntilSpace evicts files until enough space is available
func (e *Evictor) evictUntilSpace(ctx context.Context, neededBytes int64, maxCacheSize int64, maxDiskUsagePct float64) error {
	evictedCount := 0
	evictedBytes := int64(0)

	// First, clean up oversized tasks
	oversizedTasks, err := e.tasks.GetOversizedTasks(maxCacheSize)
	if err != nil {
		e.logger.Warn("failed to get oversized tasks", zap.Error(err))
	} else if len(oversizedTasks) > 0 {
		e.logger.Info("cleaning up oversized download tasks",
			zap.Int("count", len(oversizedTasks)))

		for _, task := range oversizedTasks {
			// Delete temp file if exists
			if task.TempFilePath != "" {
				if err := e.fs.DeleteTempFile(task.TempFilePath); err != nil {
					e.logger.Warn("failed to delete oversized temp file",
						zap.String("path", task.TempFilePath),
						zap.Error(err))
				}
			}

			// Delete task
			if err := e.tasks.DeleteTask(task.ID); err != nil {
				e.logger.Warn("failed to delete oversized task",
					zap.String("syno_path", task.SynoPath),
					zap.Error(err))
			} else {
				e.logger.Info("cleaned up oversized task",
					zap.String("syno_path", task.SynoPath),
					zap.Int64("size", task.Size))
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if we have enough space
		hasSpace, err := e.hasSpace(neededBytes, maxCacheSize, maxDiskUsagePct)
		if err != nil {
			return err
		}
		if hasSpace {
			e.logger.Info("eviction completed",
				zap.Int("evicted_count", evictedCount),
				zap.Int64("evicted_bytes", evictedBytes))
			return nil
		}

		// Get one candidate for eviction
		candidates, err := e.files.GetEvictionCandidates(1)
		if err != nil {
			return fmt.Errorf("failed to get eviction candidates: %w", err)
		}

		if len(candidates) == 0 {
			// Log current space situation for debugging
			cacheSize, _ := e.fs.GetCacheSize()
			usage, _ := e.fs.GetDiskUsage()
			e.logger.Warn("no eviction candidates - disk may be full with non-cache files",
				zap.Int64("cache_size_bytes", cacheSize),
				zap.Int64("max_cache_size", maxCacheSize),
				zap.Float64("disk_used_pct", usage.UsedPct),
				zap.Float64("max_disk_pct", maxDiskUsagePct),
				zap.Int64("needed_bytes", neededBytes))
			return fmt.Errorf("no eviction candidates available (cache has no files to evict)")
		}

		file := candidates[0]

		// Evict file
		if file.CachePath != "" {
			fileSize := file.Size
			if err := e.fs.DeleteFile(file.CachePath); err != nil {
				e.logger.Error("failed to delete cached file",
					zap.String("path", file.CachePath),
					zap.Error(err))
				continue
			}
			evictedBytes += fileSize
		}

		// Update database
		file.InvalidateCache()

		if err := e.files.Update(file); err != nil {
			e.logger.Error("failed to update file after eviction",
				zap.String("path", file.Path),
				zap.Error(err))
			continue
		}

		evictedCount++
		e.logger.Debug("file evicted",
			zap.String("path", file.Path),
			zap.Int("priority", file.Priority),
			zap.Int64("size", file.Size))
	}
}

// hasSpace checks if there's enough space for a file
func (e *Evictor) hasSpace(fileSize int64, maxCacheSize int64, maxDiskUsagePct float64) (bool, error) {
	cacheSize, err := e.fs.GetCacheSize()
	if err != nil {
		return false, err
	}

	if cacheSize+fileSize > maxCacheSize {
		return false, nil
	}

	usage, err := e.fs.GetDiskUsage()
	if err != nil {
		return false, err
	}

	if usage.UsedPct >= maxDiskUsagePct {
		return false, nil
	}

	newUsedPct := float64(usage.Used+uint64(fileSize)) / float64(usage.Total) * 100
	if newUsedPct >= maxDiskUsagePct {
		return false, nil
	}

	return true, nil
}
