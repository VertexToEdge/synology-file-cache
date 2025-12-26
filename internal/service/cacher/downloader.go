package cacher

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// Downloader handles file downloads
type Downloader struct {
	drive            port.DriveClient
	files            port.FileRepository
	tasks            port.DownloadTaskRepository
	fs               port.FileSystem
	logger           *zap.Logger
	maxCacheSize     int64
	progressInterval time.Duration
}

// NewDownloader creates a new Downloader
func NewDownloader(
	drive port.DriveClient,
	files port.FileRepository,
	tasks port.DownloadTaskRepository,
	fs port.FileSystem,
	logger *zap.Logger,
	maxCacheSize int64,
	progressInterval time.Duration,
) *Downloader {
	if progressInterval == 0 {
		progressInterval = 10 * time.Second
	}
	return &Downloader{
		drive:            drive,
		files:            files,
		tasks:            tasks,
		fs:               fs,
		logger:           logger,
		maxCacheSize:     maxCacheSize,
		progressInterval: progressInterval,
	}
}

// DownloadWithTask downloads a file using task for state tracking
func (d *Downloader) DownloadWithTask(ctx context.Context, file *domain.File, task *domain.DownloadTask) error {
	d.logger.Debug("downloading file",
		zap.String("path", file.Path),
		zap.Int("priority", file.Priority),
		zap.Int64("resume_from", task.BytesDownloaded))

	// Check if file size exceeds max cache size
	if file.Size > d.maxCacheSize {
		d.logger.Warn("file size exceeds max cache size",
			zap.String("path", file.Path),
			zap.Int64("file_size", file.Size),
			zap.Int64("max_cache_size", d.maxCacheSize))

		// Clean up any existing temp file
		if task.TempFilePath != "" {
			d.fs.DeleteTempFile(task.TempFilePath)
		}

		return fmt.Errorf("file size (%d bytes) exceeds max cache size (%d bytes)", file.Size, d.maxCacheSize)
	}

	// Check for resume
	var body io.ReadCloser
	var resume bool
	var tempPath string
	var err error

	if task.TempFilePath != "" && task.BytesDownloaded > 0 {
		// Verify temp file exists
		actualSize, tempMtime, err := d.fs.GetTempFileInfo(task.TempFilePath)
		if err != nil {
			d.logger.Warn("temp file not found, starting fresh",
				zap.String("path", file.Path),
				zap.Error(err))
			task.BytesDownloaded = 0
			task.TempFilePath = ""
		} else if file.ModifiedAt != nil && file.ModifiedAt.After(tempMtime) {
			d.logger.Info("file modified since last attempt, starting fresh",
				zap.String("path", file.Path))
			d.fs.DeleteTempFile(task.TempFilePath)
			task.BytesDownloaded = 0
			task.TempFilePath = ""
		} else {
			// Use actual file size for resume
			task.BytesDownloaded = actualSize
			tempPath = task.TempFilePath
			resume = true
		}
	}

	if resume {
		d.logger.Info("resuming download",
			zap.String("path", file.Path),
			zap.Int64("from_byte", task.BytesDownloaded))

		body, _, _, err = d.drive.DownloadFileWithRange(0, file.Path, task.BytesDownloaded)
		if err != nil {
			// Range request failed, try fresh download
			d.logger.Warn("resume failed, starting fresh",
				zap.String("path", file.Path),
				zap.Error(err))
			d.fs.DeleteTempFile(tempPath)
			resume = false
			task.BytesDownloaded = 0
		}
	}

	if !resume {
		body, _, _, err = d.drive.DownloadFile(0, file.Path)
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}

		tempPath = d.fs.CachePath(file.Path) + ".downloading"
		task.TempFilePath = tempPath
		task.BytesDownloaded = 0
	}
	defer body.Close()

	// Update task with temp path
	if err := d.tasks.UpdateProgress(task.ID, task.BytesDownloaded, tempPath); err != nil {
		d.logger.Warn("failed to update task progress",
			zap.String("path", file.Path),
			zap.Error(err))
	}

	// Create progress tracking wrapper
	progressReader := &progressReader{
		reader:        body,
		taskID:        task.ID,
		tasks:         d.tasks,
		tempPath:      tempPath,
		initialBytes:  task.BytesDownloaded,
		interval:      d.progressInterval,
		lastUpdate:    time.Now(),
	}

	// Write to cache
	cachePath, written, err := d.fs.WriteFileWithResume(file.Path, progressReader, resume, tempPath)
	if err != nil {
		// Update progress before returning error
		if actualSize, _, sizeErr := d.fs.GetTempFileInfo(tempPath); sizeErr == nil {
			d.tasks.UpdateProgress(task.ID, actualSize, tempPath)
		}
		return fmt.Errorf("write failed: %w", err)
	}

	// Update file as cached
	now := time.Now()
	file.MarkCached(cachePath)
	file.Size = written
	file.LastAccessInCacheAt = &now

	if err := d.files.Update(file); err != nil {
		d.fs.DeleteFile(cachePath)
		return fmt.Errorf("db update failed: %w", err)
	}

	if resume && task.BytesDownloaded > 0 {
		d.logger.Info("file cached (resumed)",
			zap.String("path", file.Path),
			zap.Int64("total_size", written),
			zap.Int64("resumed_from", task.BytesDownloaded))
	} else {
		d.logger.Info("file cached",
			zap.String("path", file.Path),
			zap.Int64("size", written))
	}

	return nil
}

// progressReader wraps a reader to report download progress
type progressReader struct {
	reader       io.Reader
	taskID       int64
	tasks        port.DownloadTaskRepository
	tempPath     string
	initialBytes int64
	bytesRead    int64
	interval     time.Duration
	lastUpdate   time.Time
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += int64(n)

	// Periodically update progress
	if time.Since(r.lastUpdate) >= r.interval {
		totalBytes := r.initialBytes + r.bytesRead
		r.tasks.UpdateProgress(r.taskID, totalBytes, r.tempPath)
		r.lastUpdate = time.Now()
	}

	return n, err
}
