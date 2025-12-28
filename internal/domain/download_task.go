package domain

import (
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain/vo"
)

// TaskState represents the state of a download task
type TaskState string

// Task status constants as TaskState
const (
	TaskStatePending    TaskState = "pending"
	TaskStateInProgress TaskState = "in_progress"
	TaskStateFailed     TaskState = "failed"
)

// Task status constants as string (for backward compatibility)
const (
	TaskStatusPending    = "pending"
	TaskStatusInProgress = "in_progress"
	TaskStatusFailed     = "failed"
)

// String returns the string representation of the task state
func (ts TaskState) String() string {
	return string(ts)
}

// IsValid returns true if the state is valid
func (ts TaskState) IsValid() bool {
	switch ts {
	case TaskStatePending, TaskStateInProgress, TaskStateFailed:
		return true
	default:
		return false
	}
}

// CanTransitionTo checks if the state can transition to the given state
func (ts TaskState) CanTransitionTo(next TaskState) bool {
	validTransitions := map[TaskState][]TaskState{
		TaskStatePending:    {TaskStateInProgress},
		TaskStateInProgress: {TaskStatePending, TaskStateFailed},
		TaskStateFailed:     {TaskStatePending},
	}
	for _, valid := range validTransitions[ts] {
		if valid == next {
			return true
		}
	}
	return false
}

// ResumeState encapsulates resume download state
type ResumeState struct {
	TempFilePath    string
	BytesDownloaded int64
}

// CanResume returns true if download can be resumed
func (rs ResumeState) CanResume() bool {
	return rs.TempFilePath != "" && rs.BytesDownloaded > 0
}

// ShouldRestart returns true if download should restart from beginning
// This happens when the source file was modified after the temp file
func (rs ResumeState) ShouldRestart(fileMTime, tempMTime time.Time) bool {
	return fileMTime.After(tempMTime)
}

// DownloadStrategy interface for download strategy pattern
type DownloadStrategy interface {
	IsFresh() bool
	ResumeFrom() int64
}

// FreshDownloadStrategy starts download from the beginning
type FreshDownloadStrategy struct{}

// IsFresh returns true for fresh download
func (s FreshDownloadStrategy) IsFresh() bool { return true }

// ResumeFrom returns 0 for fresh download
func (s FreshDownloadStrategy) ResumeFrom() int64 { return 0 }

// ResumeDownloadStrategy resumes download from a specific byte
type ResumeDownloadStrategy struct {
	FromByte int64
}

// IsFresh returns false for resume download
func (s ResumeDownloadStrategy) IsFresh() bool { return false }

// ResumeFrom returns the byte position to resume from
func (s ResumeDownloadStrategy) ResumeFrom() int64 { return s.FromByte }

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

// GetState returns the task state as a TaskState value object
func (t *DownloadTask) GetState() TaskState {
	return TaskState(t.Status)
}

// GetPriority returns the priority as a value object
func (t *DownloadTask) GetPriority() vo.Priority {
	return vo.NewPriority(t.Priority)
}

// GetSize returns the file size as a value object
func (t *DownloadTask) GetSize() vo.FileSize {
	return vo.MustFileSize(t.Size)
}

// GetPath returns the path as a value object
func (t *DownloadTask) GetPath() vo.FilePath {
	if t.SynoPath == "" {
		return vo.EmptyFilePath()
	}
	fp, _ := vo.NewFilePath(t.SynoPath)
	return fp
}

// GetResumeState returns the resume state
func (t *DownloadTask) GetResumeState() ResumeState {
	return ResumeState{
		TempFilePath:    t.TempFilePath,
		BytesDownloaded: t.BytesDownloaded,
	}
}

// IsPending returns true if task is pending
func (t *DownloadTask) IsPending() bool {
	return t.Status == string(TaskStatusPending)
}

// IsInProgress returns true if task is in progress
func (t *DownloadTask) IsInProgress() bool {
	return t.Status == string(TaskStatusInProgress)
}

// IsFailed returns true if task has failed
func (t *DownloadTask) IsFailed() bool {
	return t.Status == string(TaskStatusFailed)
}

// IsActive returns true if task is pending or in progress
func (t *DownloadTask) IsActive() bool {
	return t.IsPending() || t.IsInProgress()
}

// CanRetry returns true if the task can be retried
func (t *DownloadTask) CanRetry() bool {
	return t.RetryCount < t.MaxRetries
}

// PrepareDownload determines the download strategy based on current state
func (t *DownloadTask) PrepareDownload(fileMTime *time.Time, tempMTime time.Time) DownloadStrategy {
	resumeState := t.GetResumeState()

	// Cannot resume if no progress
	if !resumeState.CanResume() {
		return FreshDownloadStrategy{}
	}

	// Must restart if file was modified after temp file
	if fileMTime != nil && resumeState.ShouldRestart(*fileMTime, tempMTime) {
		return FreshDownloadStrategy{}
	}

	return ResumeDownloadStrategy{FromByte: resumeState.BytesDownloaded}
}

// TransitionTo attempts to transition the task to a new state
func (t *DownloadTask) TransitionTo(newState TaskState) error {
	currentState := t.GetState()
	if !currentState.CanTransitionTo(newState) {
		return ErrInvalidStateTransition
	}
	t.Status = string(newState)
	return nil
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
func (t *DownloadTask) Claim(workerID string) error {
	if err := t.TransitionTo(TaskStateInProgress); err != nil {
		// Allow claiming from pending state only
		if t.GetState() != TaskStatePending {
			return ErrTaskAlreadyClaimed
		}
	}
	t.Status = TaskStatusInProgress
	t.WorkerID = workerID
	now := time.Now()
	t.ClaimedAt = &now
	t.NextRetryAt = nil
	return nil
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

// ClearProgress clears download progress for fresh start
func (t *DownloadTask) ClearProgress() {
	t.BytesDownloaded = 0
	t.TempFilePath = ""
}

// GetRemainingBytes returns the remaining bytes to download
func (t *DownloadTask) GetRemainingBytes() int64 {
	return t.Size - t.BytesDownloaded
}

// GetProgressPercent returns the download progress as a percentage
func (t *DownloadTask) GetProgressPercent() float64 {
	if t.Size == 0 {
		return 0
	}
	return float64(t.BytesDownloaded) / float64(t.Size) * 100
}

// QueueStats represents download queue statistics
type QueueStats struct {
	PendingCount     int
	InProgressCount  int
	FailedCount      int
	TotalBytesQueued int64
}
