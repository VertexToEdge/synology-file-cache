package aggregate

import (
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/domain/event"
	"github.com/vertextoedge/synology-file-cache/internal/domain/service"
	"github.com/vertextoedge/synology-file-cache/internal/domain/vo"
)

// CacheableFile is an aggregate root that combines File and its related DownloadTask
type CacheableFile struct {
	file         *domain.File
	downloadTask *domain.DownloadTask
	events       []event.DomainEvent
}

// NewCacheableFile creates a new CacheableFile aggregate
func NewCacheableFile(file *domain.File) (*CacheableFile, error) {
	if file == nil {
		return nil, domain.ErrNilFile
	}
	return &CacheableFile{
		file:   file,
		events: make([]event.DomainEvent, 0),
	}, nil
}

// NewCacheableFileWithTask creates a new CacheableFile aggregate with a download task
func NewCacheableFileWithTask(file *domain.File, task *domain.DownloadTask) (*CacheableFile, error) {
	cf, err := NewCacheableFile(file)
	if err != nil {
		return nil, err
	}
	cf.downloadTask = task
	return cf, nil
}

// File returns the underlying file
func (cf *CacheableFile) File() *domain.File {
	return cf.file
}

// DownloadTask returns the associated download task (may be nil)
func (cf *CacheableFile) DownloadTask() *domain.DownloadTask {
	return cf.downloadTask
}

// HasActiveTask returns true if there's an active download task
func (cf *CacheableFile) HasActiveTask() bool {
	return cf.downloadTask != nil && cf.downloadTask.IsActive()
}

// IsCached returns true if the file is cached
func (cf *CacheableFile) IsCached() bool {
	return cf.file.IsCached()
}

// RequestDownload creates a download task for this file if eligible
func (cf *CacheableFile) RequestDownload(policy *service.CachePolicy, maxRetries int) (*domain.DownloadTask, error) {
	// Check if file can be cached
	if err := policy.CanCache(cf.file); err != nil {
		return nil, err
	}

	// Check if there's already an active task
	if cf.HasActiveTask() {
		return nil, domain.ErrTaskAlreadyExists
	}

	// Create new download task
	task := &domain.DownloadTask{
		FileID:     cf.file.ID,
		SynoPath:   cf.file.Path,
		Priority:   cf.file.Priority,
		Size:       cf.file.Size,
		Status:     string(domain.TaskStatusPending),
		MaxRetries: maxRetries,
		CreatedAt:  time.Now(),
	}

	cf.downloadTask = task

	// Record event
	cf.addEvent(event.NewDownloadTaskCreated(
		task.ID,
		task.FileID,
		task.SynoPath,
		task.Size,
		task.Priority,
	))

	return task, nil
}

// MarkCached marks the file as cached and clears the download task
func (cf *CacheableFile) MarkCached(cachePath string, bytesWritten int64, resumed bool) error {
	if cf.file.IsCached() {
		return domain.ErrFileAlreadyCached
	}

	cf.file.MarkCached(cachePath)

	// Record event
	cf.addEvent(event.NewFileDownloaded(
		cf.file.ID,
		cf.file.Path,
		cachePath,
		bytesWritten,
		resumed,
	))

	// Clear the download task on success
	cf.downloadTask = nil

	return nil
}

// Invalidate invalidates the file's cache
func (cf *CacheableFile) Invalidate(reason string) {
	if !cf.file.IsCached() {
		return
	}

	cf.file.InvalidateCache()

	// Record event
	cf.addEvent(event.NewFileCacheInvalidated(
		cf.file.ID,
		cf.file.Path,
		reason,
	))
}

// Evict evicts the file from cache
func (cf *CacheableFile) Evict() {
	if !cf.file.IsCached() {
		return
	}

	cachePath := cf.file.CachePath
	size := cf.file.Size
	priority := cf.file.Priority

	cf.file.InvalidateCache()

	// Record event
	cf.addEvent(event.NewFileEvicted(
		cf.file.ID,
		cachePath,
		size,
		priority,
	))
}

// UpdatePriority updates the file priority based on category
func (cf *CacheableFile) UpdatePriority(calculator *service.PriorityCalculator, category service.FileCategory) bool {
	return calculator.UpdateFilePriority(cf.file, category)
}

// UpdatePriorityVO updates the file priority using a Priority value object
func (cf *CacheableFile) UpdatePriorityVO(priority vo.Priority) bool {
	return cf.file.UpdatePriorityVO(priority)
}

// NeedsRedownload checks if the file needs to be re-downloaded based on mtime
func (cf *CacheableFile) NeedsRedownload(newMTime time.Time) bool {
	return cf.file.NeedsRedownload(newMTime)
}

// RecordAccess records an access to the cached file
func (cf *CacheableFile) RecordAccess() {
	cf.file.RecordAccess()
}

// GetPendingEvents returns all pending domain events
func (cf *CacheableFile) GetPendingEvents() []event.DomainEvent {
	return cf.events
}

// ClearEvents clears all pending domain events
func (cf *CacheableFile) ClearEvents() {
	cf.events = make([]event.DomainEvent, 0)
}

// addEvent adds an event to the pending events
func (cf *CacheableFile) addEvent(e event.DomainEvent) {
	cf.events = append(cf.events, e)
}

// SetShared sets the shared status
func (cf *CacheableFile) SetShared(shared bool) {
	cf.file.SetShared(shared)
}

// SetStarred sets the starred status
func (cf *CacheableFile) SetStarred(starred bool) {
	cf.file.SetStarred(starred)
}

// CanBeCached checks if the file can be cached with the given policy
func (cf *CacheableFile) CanBeCached(policy *service.CachePolicy) error {
	return policy.CanCache(cf.file)
}

// PrepareDownloadStrategy determines the download strategy for the current task
func (cf *CacheableFile) PrepareDownloadStrategy(fileMTime *time.Time, tempMTime time.Time) domain.DownloadStrategy {
	if cf.downloadTask == nil {
		return domain.FreshDownloadStrategy{}
	}
	return cf.downloadTask.PrepareDownload(fileMTime, tempMTime)
}
