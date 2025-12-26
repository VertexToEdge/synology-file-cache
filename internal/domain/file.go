package domain

import (
	"time"
)

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
