package scanner

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/store"
	"github.com/vertextoedge/synology-file-cache/internal/synoapi"
	"go.uber.org/zap"
)

// Config holds scanner configuration
type Config struct {
	MaxConcurrency int // Maximum concurrent API requests
	BatchSize      int // Number of files to list per API call
}

// DefaultConfig returns default scanner configuration
func DefaultConfig() *Config {
	return &Config{
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
	config *Config
	client *synoapi.Client
	store  *store.Store
	logger *zap.Logger

	// Worker pool
	sem chan struct{}

	// Stats
	stats struct {
		totalFiles   atomic.Int64
		addedFiles   atomic.Int64
		updatedFiles atomic.Int64
		errors       atomic.Int64
	}
}

// New creates a new Scanner
func New(cfg *Config, client *synoapi.Client, st *store.Store, logger *zap.Logger) *Scanner {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.MaxConcurrency < 1 {
		cfg.MaxConcurrency = 1
	}
	if cfg.BatchSize < 1 {
		cfg.BatchSize = 200
	}

	return &Scanner{
		config: cfg,
		client: client,
		store:  st,
		logger: logger,
		sem:    make(chan struct{}, cfg.MaxConcurrency),
	}
}

// ScanPath scans a path recursively and adds all files to the database
// Files are added with the specified priority
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

	// Use WaitGroup to track all goroutines
	var wg sync.WaitGroup

	// Start scanning
	if err := s.scanDir(ctx, path, priority, &wg); err != nil {
		return nil, fmt.Errorf("failed to scan path %s: %w", path, err)
	}

	// Wait for all goroutines to complete
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

// ScanPathAsync scans a path in the background and returns immediately
// The result can be retrieved via the returned channel
func (s *Scanner) ScanPathAsync(ctx context.Context, path string, priority int) <-chan *ScanResult {
	resultCh := make(chan *ScanResult, 1)

	go func() {
		defer close(resultCh)

		result, err := s.ScanPath(ctx, path, priority)
		if err != nil {
			s.logger.Error("async scan failed",
				zap.String("path", path),
				zap.Error(err))
			resultCh <- &ScanResult{Errors: 1}
			return
		}
		resultCh <- result
	}()

	return resultCh
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

		files, err := s.client.DriveListFiles(&synoapi.DriveListOptions{
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

// processFile adds or updates a file in the database
func (s *Scanner) processFile(ctx context.Context, file *synoapi.DriveFile, priority int, now *time.Time) error {
	fileID := file.ID.String()

	existing, err := s.store.GetFileBySynoID(fileID)
	if err != nil {
		return fmt.Errorf("failed to check existing file: %w", err)
	}

	if existing != nil {
		// Update existing file
		existing.Path = file.Path
		existing.Size = file.Size
		existing.LastSyncAt = now

		// Only lower priority, don't raise it
		if priority < existing.Priority {
			existing.Priority = priority
		}

		if file.MTime > 0 {
			mtime := time.Unix(file.MTime, 0)
			existing.ModifiedAt = &mtime
		}

		if err := s.store.UpdateFile(existing); err != nil {
			return fmt.Errorf("failed to update file: %w", err)
		}

		s.stats.updatedFiles.Add(1)
	} else {
		// Create new file
		newFile := &store.File{
			SynoFileID: fileID,
			Path:       file.Path,
			Size:       file.Size,
			Starred:    file.Starred,
			Shared:     file.Shared,
			Priority:   priority,
			LastSyncAt: now,
		}

		if file.MTime > 0 {
			mtime := time.Unix(file.MTime, 0)
			newFile.ModifiedAt = &mtime
		}
		if file.ATime > 0 {
			atime := time.Unix(file.ATime, 0)
			newFile.AccessedAt = &atime
		}

		if err := s.store.CreateFile(newFile); err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}

		s.stats.addedFiles.Add(1)

		s.logger.Debug("file added from scan",
			zap.String("path", file.Path),
			zap.Int("priority", priority))
	}

	return nil
}
