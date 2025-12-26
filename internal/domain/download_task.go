package domain

import "time"

// Task status constants
const (
	TaskStatusPending    = "pending"
	TaskStatusInProgress = "in_progress"
	TaskStatusFailed     = "failed"
)

// Default retry backoffs
var defaultRetryBackoffs = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
}

// DownloadTask represents a download task in the queue
type DownloadTask struct {
	ID       int64
	FileID   int64
	SynoPath string
	Priority int
	Size     int64

	// State
	Status   string
	WorkerID string

	// Resume support
	TempFilePath    string
	BytesDownloaded int64

	// Retry handling
	RetryCount  int
	MaxRetries  int
	NextRetryAt *time.Time
	LastError   string

	// Timestamps
	CreatedAt time.Time
	ClaimedAt *time.Time
	UpdatedAt time.Time
}

// CanRetry returns true if the task can be retried
func (t *DownloadTask) CanRetry() bool {
	return t.RetryCount < t.MaxRetries
}

// MarkFailed marks the task as failed with an error message
// If retries are available, schedules a retry with exponential backoff
func (t *DownloadTask) MarkFailed(err string) {
	t.RetryCount++
	t.LastError = err
	t.WorkerID = ""
	t.ClaimedAt = nil

	if t.CanRetry() {
		t.Status = TaskStatusPending
		backoffIdx := t.RetryCount - 1
		if backoffIdx >= len(defaultRetryBackoffs) {
			backoffIdx = len(defaultRetryBackoffs) - 1
		}
		nextRetry := time.Now().Add(defaultRetryBackoffs[backoffIdx])
		t.NextRetryAt = &nextRetry
	} else {
		t.Status = TaskStatusFailed
	}
}

// Claim marks the task as claimed by a worker
func (t *DownloadTask) Claim(workerID string) {
	t.Status = TaskStatusInProgress
	t.WorkerID = workerID
	now := time.Now()
	t.ClaimedAt = &now
	t.NextRetryAt = nil
}

// UpdateProgress updates the download progress
func (t *DownloadTask) UpdateProgress(bytesDownloaded int64, tempPath string) {
	t.BytesDownloaded = bytesDownloaded
	if tempPath != "" {
		t.TempFilePath = tempPath
	}
}

// ResetForRetry resets the task for a fresh retry attempt
func (t *DownloadTask) ResetForRetry() {
	t.Status = TaskStatusPending
	t.WorkerID = ""
	t.ClaimedAt = nil
	t.NextRetryAt = nil
}

// QueueStats represents download queue statistics
type QueueStats struct {
	PendingCount     int
	InProgressCount  int
	FailedCount      int
	TotalBytesQueued int64
}
