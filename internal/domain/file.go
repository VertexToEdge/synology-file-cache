package domain

import (
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain/vo"
)

// CacheState encapsulates caching state and behavior
type CacheState struct {
	Cached              bool
	CachePath           string
	LastAccessInCacheAt *time.Time
}

// IsEmpty returns true if cache state is empty/not cached
func (cs CacheState) IsEmpty() bool {
	return !cs.Cached && cs.CachePath == ""
}

// FileAttributes encapsulates file classification attributes
type FileAttributes struct {
	Starred bool
	Shared  bool
	Labels  []string
}

// File represents a file in the cache system
type File struct {
	ID                  int64
	SynoFileID          string
	Path                string
	Size                int64
	ModifiedAt          *time.Time
	AccessedAt          *time.Time
	Starred             bool
	Shared              bool
	LastSyncAt          *time.Time
	Cached              bool
	CachePath           string
	Priority            int
	LastAccessInCacheAt *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// GetCacheState returns the cache state of the file
func (f *File) GetCacheState() CacheState {
	return CacheState{
		Cached:              f.Cached,
		CachePath:           f.CachePath,
		LastAccessInCacheAt: f.LastAccessInCacheAt,
	}
}

// GetAttributes returns the file attributes
func (f *File) GetAttributes() FileAttributes {
	return FileAttributes{
		Starred: f.Starred,
		Shared:  f.Shared,
	}
}

// GetSize returns the file size as a value object
func (f *File) GetSize() vo.FileSize {
	return vo.MustFileSize(f.Size)
}

// GetPriority returns the priority as a value object
func (f *File) GetPriority() vo.Priority {
	return vo.NewPriority(f.Priority)
}

// GetPath returns the path as a value object
func (f *File) GetPath() vo.FilePath {
	if f.Path == "" {
		return vo.EmptyFilePath()
	}
	fp, _ := vo.NewFilePath(f.Path)
	return fp
}

// ShouldInvalidateCache checks if the file should be invalidated based on new mtime
func (f *File) ShouldInvalidateCache(newMTime time.Time) bool {
	if !f.Cached {
		return false
	}
	if f.ModifiedAt == nil {
		return false
	}
	return newMTime.After(*f.ModifiedAt)
}

// InvalidateCache marks the file as not cached
func (f *File) InvalidateCache() {
	f.Cached = false
	f.CachePath = ""
}

// MarkCached marks the file as cached with the given path
func (f *File) MarkCached(cachePath string) {
	f.Cached = true
	f.CachePath = cachePath
	now := time.Now()
	f.LastAccessInCacheAt = &now
}

// UpdatePriority updates the priority if the new priority is higher (lower number)
func (f *File) UpdatePriority(newPriority int) bool {
	if newPriority < f.Priority {
		f.Priority = newPriority
		return true
	}
	return false
}

// UpdatePriorityVO updates the priority using a Priority value object
func (f *File) UpdatePriorityVO(newPriority vo.Priority) bool {
	return f.UpdatePriority(newPriority.Value())
}

// CanBeCached checks if the file can be cached based on size limit
func (f *File) CanBeCached(maxSize vo.FileSize) error {
	if f.Cached {
		return ErrFileAlreadyCached
	}
	if f.GetSize().ExceedsLimit(maxSize) {
		return ErrFileTooLarge
	}
	return nil
}

// NeedsRedownload checks if the file needs to be re-downloaded
func (f *File) NeedsRedownload(newMTime time.Time) bool {
	return f.ShouldInvalidateCache(newMTime)
}

// RecordAccess updates the last access time
func (f *File) RecordAccess() {
	now := time.Now()
	f.LastAccessInCacheAt = &now
}

// IsRecentlyModified checks if the file was modified within the threshold
func (f *File) IsRecentlyModified(threshold time.Duration) bool {
	if f.ModifiedAt == nil {
		return false
	}
	return time.Since(*f.ModifiedAt) <= threshold
}

// IsRecentlyAccessed checks if the file was accessed within the threshold
func (f *File) IsRecentlyAccessed(threshold time.Duration) bool {
	if f.AccessedAt == nil {
		return false
	}
	return time.Since(*f.AccessedAt) <= threshold
}

// IsShared returns true if the file is shared
func (f *File) IsShared() bool {
	return f.Shared
}

// IsStarred returns true if the file is starred
func (f *File) IsStarred() bool {
	return f.Starred
}

// IsCached returns true if the file is cached
func (f *File) IsCached() bool {
	return f.Cached
}

// SetShared sets the shared status
func (f *File) SetShared(shared bool) {
	f.Shared = shared
}

// SetStarred sets the starred status
func (f *File) SetStarred(starred bool) {
	f.Starred = starred
}

// ExceedsMaxSize checks if the file exceeds the given maximum size
func (f *File) ExceedsMaxSize(maxSize int64) bool {
	return maxSize > 0 && f.Size > maxSize
}

// TempFile represents a temporary download file
type TempFile struct {
	ID             int64
	SynoPath       string
	TempFilePath   string
	SizeDownloaded int64
	StartedAt      time.Time
	UpdatedAt      time.Time
}

// CacheStats represents cache statistics
type CacheStats struct {
	TotalFiles      int64
	CachedFiles     int64
	CachedSizeBytes int64
	ActiveShares    int64
}
