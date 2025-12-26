package syncer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// ScannerConfig holds scanner configuration
type ScannerConfig struct {
	MaxConcurrency int
	BatchSize      int
}

// DefaultScannerConfig returns default scanner configuration
func DefaultScannerConfig() *ScannerConfig {
	return &ScannerConfig{
		MaxConcurrency: 3,
		BatchSize:      200,
	}
}

// ScanResult holds the result of a path scan
type ScanResult struct {
	TotalFiles   int
	AddedFiles   int
	UpdatedFiles int
	Errors       int
	Duration     time.Duration
}

// Scanner recursively scans paths and adds files to the database
type Scanner struct {
	config *ScannerConfig
	drive  port.DriveClient
	files  port.FileRepository
	tasks  port.DownloadTaskRepository
	logger *zap.Logger
	sem    chan struct{}

	// Stats
	stats struct {
		totalFiles   atomic.Int64
		addedFiles   atomic.Int64
		updatedFiles atomic.Int64
		errors       atomic.Int64
	}
}

// NewScanner creates a new Scanner
func NewScanner(cfg *ScannerConfig, drive port.DriveClient, files port.FileRepository, tasks port.DownloadTaskRepository, logger *zap.Logger) *Scanner {
	if cfg == nil {
		cfg = DefaultScannerConfig()
	}
	if cfg.MaxConcurrency < 1 {
		cfg.MaxConcurrency = 1
	}
	if cfg.BatchSize < 1 {
		cfg.BatchSize = 200
	}

	return &Scanner{
		config: cfg,
		drive:  drive,
		files:  files,
		tasks:  tasks,
		logger: logger,
		sem:    make(chan struct{}, cfg.MaxConcurrency),
	}
}

// ScanPath scans a path recursively and adds all files to the database
func (s *Scanner) ScanPath(ctx context.Context, path string, priority int) (*ScanResult, error) {
	start := time.Now()

	// Reset stats
	s.stats.totalFiles.Store(0)
	s.stats.addedFiles.Store(0)
	s.stats.updatedFiles.Store(0)
	s.stats.errors.Store(0)

	s.logger.Info("starting path scan",
		zap.String("path", path),
		zap.Int("priority", priority),
		zap.Int("max_concurrency", s.config.MaxConcurrency))

	var wg sync.WaitGroup

	if err := s.scanDir(ctx, path, priority, &wg); err != nil {
		return nil, fmt.Errorf("failed to scan path %s: %w", path, err)
	}

	wg.Wait()

	result := &ScanResult{
		TotalFiles:   int(s.stats.totalFiles.Load()),
		AddedFiles:   int(s.stats.addedFiles.Load()),
		UpdatedFiles: int(s.stats.updatedFiles.Load()),
		Errors:       int(s.stats.errors.Load()),
		Duration:     time.Since(start),
	}

	s.logger.Info("path scan completed",
		zap.String("path", path),
		zap.Duration("duration", result.Duration),
		zap.Int("total", result.TotalFiles),
		zap.Int("added", result.AddedFiles),
		zap.Int("updated", result.UpdatedFiles),
		zap.Int("errors", result.Errors))

	return result, nil
}

// scanDir scans a directory and its subdirectories
func (s *Scanner) scanDir(ctx context.Context, path string, priority int, wg *sync.WaitGroup) error {
	offset := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Acquire semaphore for API call
		select {
		case s.sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}

		files, err := s.drive.ListFiles(&port.DriveListOptions{
			Path:   path,
			Offset: offset,
			Limit:  s.config.BatchSize,
		})

		// Release semaphore
		<-s.sem

		if err != nil {
			s.stats.errors.Add(1)
			return fmt.Errorf("failed to list %s at offset %d: %w", path, offset, err)
		}

		if len(files.Items) == 0 {
			break
		}

		now := time.Now()

		for _, file := range files.Items {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if file.IsDir() {
				// Scan subdirectory in a new goroutine
				wg.Add(1)
				go func(dirPath string) {
					defer wg.Done()
					if err := s.scanDir(ctx, dirPath, priority, wg); err != nil {
						s.logger.Warn("failed to scan subdirectory",
							zap.String("path", dirPath),
							zap.Error(err))
						s.stats.errors.Add(1)
					}
				}(file.Path)
				continue
			}

			// Process file
			s.stats.totalFiles.Add(1)
			if err := s.processFile(ctx, &file, priority, &now); err != nil {
				s.logger.Warn("failed to process file",
					zap.String("path", file.Path),
					zap.Error(err))
				s.stats.errors.Add(1)
			}
		}

		offset += len(files.Items)
		if offset >= files.Total || len(files.Items) < s.config.BatchSize {
			break
		}
	}

	return nil
}

// processFile adds or updates a file in the database and enqueues download task
func (s *Scanner) processFile(ctx context.Context, file *port.DriveFile, priority int, now *time.Time) error {
	fileID := file.GetIDString()

	existing, err := s.files.GetBySynoID(fileID)
	if err != nil {
		return fmt.Errorf("failed to check existing file: %w", err)
	}

	var dbFile *domain.File
	var isNew bool

	if existing != nil {
		dbFile = existing

		// Update existing file metadata
		existing.Path = file.Path
		existing.Size = file.Size
		existing.LastSyncAt = now

		// Only lower priority, don't raise it
		existing.UpdatePriority(priority)

		if mtime := file.GetMTime(); mtime != nil {
			existing.ModifiedAt = mtime
		}

		// Use UpdateMetadata to avoid overwriting cache status set by cacher
		if err := s.files.UpdateMetadata(existing); err != nil {
			return fmt.Errorf("failed to update file: %w", err)
		}

		s.stats.updatedFiles.Add(1)
	} else {
		isNew = true

		// Create new file
		newFile := &domain.File{
			SynoFileID: fileID,
			Path:       file.Path,
			Size:       file.Size,
			Starred:    file.Starred,
			Shared:     file.Shared,
			Priority:   priority,
			LastSyncAt: now,
		}

		if mtime := file.GetMTime(); mtime != nil {
			newFile.ModifiedAt = mtime
		}
		if atime := file.GetATime(); atime != nil {
			newFile.AccessedAt = atime
		}

		if err := s.files.Create(newFile); err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}

		dbFile = newFile
		s.stats.addedFiles.Add(1)

		s.logger.Debug("file added from scan",
			zap.String("path", file.Path),
			zap.Int("priority", priority))
	}

	// Enqueue download task if file needs caching
	if isNew || !dbFile.Cached {
		s.enqueueDownloadTask(dbFile)
	}

	return nil
}

// enqueueDownloadTask creates a download task for a file if one doesn't exist
func (s *Scanner) enqueueDownloadTask(file *domain.File) {
	// Re-read file to get latest cached status (avoid race with cacher)
	latestFile, err := s.files.GetByID(file.ID)
	if err != nil {
		s.logger.Warn("failed to re-read file for task enqueue",
			zap.String("path", file.Path),
			zap.Error(err))
		return
	}
	if latestFile == nil || latestFile.Cached {
		// File not found or already cached
		return
	}

	// Check if task already exists
	hasTask, err := s.tasks.HasActiveTask(file.ID)
	if err != nil {
		s.logger.Warn("failed to check existing task",
			zap.String("path", file.Path),
			zap.Error(err))
		return
	}

	if hasTask {
		return
	}

	task := &domain.DownloadTask{
		FileID:     file.ID,
		SynoPath:   file.Path,
		Priority:   file.Priority,
		Size:       file.Size,
		Status:     domain.TaskStatusPending,
		MaxRetries: 3,
	}

	if err := s.tasks.CreateTask(task); err != nil {
		if err != domain.ErrAlreadyExists {
			s.logger.Warn("failed to create download task",
				zap.String("path", file.Path),
				zap.Error(err))
		}
	}
}
