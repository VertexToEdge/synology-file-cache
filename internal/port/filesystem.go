package port

import (
	"io"
	"time"
)

// DiskUsage represents disk usage statistics
type DiskUsage struct {
	Total   uint64  // Total disk space in bytes
	Used    uint64  // Used disk space in bytes
	Free    uint64  // Free disk space in bytes
	UsedPct float64 // Used percentage (0-100)
}

// FileSystem defines the interface for filesystem operations
type FileSystem interface {
	// RootDir returns the cache root directory
	RootDir() string

	// CachePath returns the local cache path for a Synology file path
	CachePath(synoPath string) string

	// WriteFile writes content to the cache
	// Returns: cache path, bytes written, error
	WriteFile(synoPath string, reader io.Reader) (string, int64, error)

	// WriteFileWithResume writes content with optional resume support
	// If resume is true and tempPath exists, it will append to it
	// Returns: cache path, total bytes written, error
	WriteFileWithResume(synoPath string, reader io.Reader, resume bool, tempPath string) (string, int64, error)

	// DeleteFile removes a cached file
	DeleteFile(cachePath string) error

	// FileExists checks if a cached file exists
	FileExists(cachePath string) bool

	// GetFileSize returns the size of a cached file
	GetFileSize(cachePath string) (int64, error)

	// GetTempFileInfo returns size and modification time of a temp file
	// Returns (0, zero time, nil) if the file doesn't exist
	GetTempFileInfo(tempPath string) (int64, time.Time, error)

	// DeleteTempFile removes a temporary file
	DeleteTempFile(tempPath string) error

	// GetCacheSize returns total size of cached files
	GetCacheSize() (int64, error)

	// GetDiskUsage returns disk usage statistics
	GetDiskUsage() (*DiskUsage, error)

	// CleanOldTempFiles removes temp files older than the specified duration
	// Returns the number of files deleted
	CleanOldTempFiles(olderThan time.Duration) (int, error)
}
