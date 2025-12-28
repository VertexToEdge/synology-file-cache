package event

import (
	"time"
)

// DomainEvent is the interface for all domain events
type DomainEvent interface {
	// EventName returns the name of the event
	EventName() string
	// OccurredAt returns when the event occurred
	OccurredAt() time.Time
}

// BaseEvent provides common fields for all events
type BaseEvent struct {
	Timestamp time.Time
}

// OccurredAt returns when the event occurred
func (e BaseEvent) OccurredAt() time.Time {
	return e.Timestamp
}

// FileDownloaded is raised when a file is successfully downloaded and cached
type FileDownloaded struct {
	BaseEvent
	FileID    int64
	SynoPath  string
	CachePath string
	Size      int64
	Resumed   bool
}

// EventName returns the event name
func (e FileDownloaded) EventName() string {
	return "file.downloaded"
}

// NewFileDownloaded creates a new FileDownloaded event
func NewFileDownloaded(fileID int64, synoPath, cachePath string, size int64, resumed bool) FileDownloaded {
	return FileDownloaded{
		BaseEvent: BaseEvent{Timestamp: time.Now()},
		FileID:    fileID,
		SynoPath:  synoPath,
		CachePath: cachePath,
		Size:      size,
		Resumed:   resumed,
	}
}

// FileCacheInvalidated is raised when a file's cache is invalidated
type FileCacheInvalidated struct {
	BaseEvent
	FileID   int64
	SynoPath string
	Reason   string
}

// EventName returns the event name
func (e FileCacheInvalidated) EventName() string {
	return "file.cache_invalidated"
}

// NewFileCacheInvalidated creates a new FileCacheInvalidated event
func NewFileCacheInvalidated(fileID int64, synoPath, reason string) FileCacheInvalidated {
	return FileCacheInvalidated{
		BaseEvent: BaseEvent{Timestamp: time.Now()},
		FileID:    fileID,
		SynoPath:  synoPath,
		Reason:    reason,
	}
}

// FileEvicted is raised when a file is evicted from cache
type FileEvicted struct {
	BaseEvent
	FileID    int64
	CachePath string
	Size      int64
	Priority  int
}

// EventName returns the event name
func (e FileEvicted) EventName() string {
	return "file.evicted"
}

// NewFileEvicted creates a new FileEvicted event
func NewFileEvicted(fileID int64, cachePath string, size int64, priority int) FileEvicted {
	return FileEvicted{
		BaseEvent: BaseEvent{Timestamp: time.Now()},
		FileID:    fileID,
		CachePath: cachePath,
		Size:      size,
		Priority:  priority,
	}
}

// ShareAccessed is raised when a shared file is accessed
type ShareAccessed struct {
	BaseEvent
	ShareID  int64
	FileID   int64
	Token    string
	ClientIP string
}

// EventName returns the event name
func (e ShareAccessed) EventName() string {
	return "share.accessed"
}

// NewShareAccessed creates a new ShareAccessed event
func NewShareAccessed(shareID, fileID int64, token, clientIP string) ShareAccessed {
	return ShareAccessed{
		BaseEvent: BaseEvent{Timestamp: time.Now()},
		ShareID:   shareID,
		FileID:    fileID,
		Token:     token,
		ClientIP:  clientIP,
	}
}

// DownloadTaskCreated is raised when a download task is created
type DownloadTaskCreated struct {
	BaseEvent
	TaskID   int64
	FileID   int64
	SynoPath string
	Size     int64
	Priority int
}

// EventName returns the event name
func (e DownloadTaskCreated) EventName() string {
	return "download_task.created"
}

// NewDownloadTaskCreated creates a new DownloadTaskCreated event
func NewDownloadTaskCreated(taskID, fileID int64, synoPath string, size int64, priority int) DownloadTaskCreated {
	return DownloadTaskCreated{
		BaseEvent: BaseEvent{Timestamp: time.Now()},
		TaskID:    taskID,
		FileID:    fileID,
		SynoPath:  synoPath,
		Size:      size,
		Priority:  priority,
	}
}

// DownloadTaskFailed is raised when a download task fails
type DownloadTaskFailed struct {
	BaseEvent
	TaskID     int64
	FileID     int64
	SynoPath   string
	Error      string
	RetryCount int
	CanRetry   bool
}

// EventName returns the event name
func (e DownloadTaskFailed) EventName() string {
	return "download_task.failed"
}

// NewDownloadTaskFailed creates a new DownloadTaskFailed event
func NewDownloadTaskFailed(taskID, fileID int64, synoPath, err string, retryCount int, canRetry bool) DownloadTaskFailed {
	return DownloadTaskFailed{
		BaseEvent:  BaseEvent{Timestamp: time.Now()},
		TaskID:     taskID,
		FileID:     fileID,
		SynoPath:   synoPath,
		Error:      err,
		RetryCount: retryCount,
		CanRetry:   canRetry,
	}
}

// DownloadTaskCompleted is raised when a download task completes
type DownloadTaskCompleted struct {
	BaseEvent
	TaskID    int64
	FileID    int64
	SynoPath  string
	CachePath string
	Size      int64
	Duration  time.Duration
}

// EventName returns the event name
func (e DownloadTaskCompleted) EventName() string {
	return "download_task.completed"
}

// NewDownloadTaskCompleted creates a new DownloadTaskCompleted event
func NewDownloadTaskCompleted(taskID, fileID int64, synoPath, cachePath string, size int64, duration time.Duration) DownloadTaskCompleted {
	return DownloadTaskCompleted{
		BaseEvent: BaseEvent{Timestamp: time.Now()},
		TaskID:    taskID,
		FileID:    fileID,
		SynoPath:  synoPath,
		CachePath: cachePath,
		Size:      size,
		Duration:  duration,
	}
}

// SyncCompleted is raised when a sync operation completes
type SyncCompleted struct {
	BaseEvent
	SyncType   string // "full" or "incremental"
	FilesAdded int
	Duration   time.Duration
}

// EventName returns the event name
func (e SyncCompleted) EventName() string {
	return "sync.completed"
}

// NewSyncCompleted creates a new SyncCompleted event
func NewSyncCompleted(syncType string, filesAdded int, duration time.Duration) SyncCompleted {
	return SyncCompleted{
		BaseEvent:  BaseEvent{Timestamp: time.Now()},
		SyncType:   syncType,
		FilesAdded: filesAdded,
		Duration:   duration,
	}
}
