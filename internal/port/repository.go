package port

import (
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
)

// FileRepository defines the interface for file persistence operations
type FileRepository interface {
	// GetByID retrieves a file by its internal ID
	GetByID(id int64) (*domain.File, error)

	// GetBySynoID retrieves a file by its Synology file ID
	GetBySynoID(synoID string) (*domain.File, error)

	// GetByPath retrieves a file by its path
	GetByPath(path string) (*domain.File, error)

	// Create creates a new file record
	Create(file *domain.File) error

	// Update updates an existing file record (including cache status)
	// Use UpdateMetadata for syncer to avoid overwriting cache status
	Update(file *domain.File) error

	// UpdateMetadata updates file metadata without touching cache status fields
	// This prevents race condition between syncer and cacher
	UpdateMetadata(file *domain.File) error

	// InvalidateCache sets cached=false for a file (used when source file is modified)
	InvalidateCache(fileID int64) error

	// Delete deletes a file record by ID
	Delete(id int64) error

	// GetEvictionCandidates returns cached files that can be evicted
	// Files are ordered by priority (lowest first) and then by LRU
	GetEvictionCandidates(limit int) ([]*domain.File, error)
}

// ShareRepository defines the interface for share persistence operations
type ShareRepository interface {
	// GetShareByToken retrieves a share by its token
	GetShareByToken(token string) (*domain.Share, error)

	// GetFileByShareToken retrieves both the file and share by share token
	GetFileByShareToken(token string) (*domain.File, *domain.Share, error)

	// CreateShare creates a new share record
	CreateShare(share *domain.Share) error

	// UpdateShare updates an existing share record
	UpdateShare(share *domain.Share) error
}

// DownloadTaskRepository defines the interface for download task queue operations
type DownloadTaskRepository interface {
	// CreateTask creates a new download task
	// Returns domain.ErrAlreadyExists if an active task for this file already exists
	CreateTask(task *domain.DownloadTask) error

	// ClaimNextTask atomically claims the next pending task for a worker
	// Returns nil if no tasks are available
	// Respects priority ordering (priority ASC, size ASC)
	// Only claims tasks where next_retry_at is NULL or <= now
	ClaimNextTask(workerID string) (*domain.DownloadTask, error)

	// GetTask retrieves a task by ID
	GetTask(id int64) (*domain.DownloadTask, error)

	// GetTaskByFileID retrieves an active task for a file
	GetTaskByFileID(fileID int64) (*domain.DownloadTask, error)

	// HasActiveTask checks if a file has an active (pending or in_progress) task
	HasActiveTask(fileID int64) (bool, error)

	// UpdateTask updates a task's state
	UpdateTask(task *domain.DownloadTask) error

	// UpdateProgress updates download progress (bytes_downloaded, temp_file_path)
	UpdateProgress(taskID int64, bytesDownloaded int64, tempPath string) error

	// CompleteTask removes a completed task
	CompleteTask(taskID int64) error

	// FailTask marks a task as failed and schedules retry if possible
	FailTask(taskID int64, errMsg string, canRetry bool) error

	// ReleaseStaleInProgressTasks resets tasks stuck in in_progress state
	// Used for tasks where worker died (claimed_at older than timeout)
	ReleaseStaleInProgressTasks(staleDuration time.Duration) (int, error)

	// GetQueueStats returns queue statistics
	GetQueueStats() (*domain.QueueStats, error)

	// CleanupOldFailedTasks removes failed tasks older than the specified duration
	CleanupOldFailedTasks(olderThan time.Duration) (int, error)

	// GetOversizedTasks returns tasks whose file size exceeds maxSize
	GetOversizedTasks(maxSize int64) ([]*domain.DownloadTask, error)

	// DeleteTask removes a task by ID
	DeleteTask(taskID int64) error
}

// StatsRepository defines the interface for cache statistics
type StatsRepository interface {
	// GetCacheStats returns cache statistics
	GetCacheStats() (*domain.CacheStats, error)
}

// Store combines all repository interfaces
type Store interface {
	FileRepository
	ShareRepository
	DownloadTaskRepository
	StatsRepository

	// Close closes the database connection
	Close() error

	// Ping checks database connectivity
	Ping() error
}
