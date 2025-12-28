package repository

import (
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
)

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
