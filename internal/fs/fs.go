package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Manager handles local filesystem operations
type Manager struct {
	rootDir string
}

// NewManager creates a new filesystem manager
func NewManager(rootDir string) (*Manager, error) {
	// Ensure root directory exists
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache root dir: %w", err)
	}

	return &Manager{
		rootDir: rootDir,
	}, nil
}

// RootDir returns the cache root directory
func (m *Manager) RootDir() string {
	return m.rootDir
}

// DiskUsage represents disk usage statistics
type DiskUsage struct {
	Total   uint64  // Total disk space in bytes
	Used    uint64  // Used disk space in bytes
	Free    uint64  // Free disk space in bytes
	UsedPct float64 // Used percentage (0-100)
}

// GetDiskUsage returns disk usage for the cache directory
// Platform-specific implementation in fs_unix.go and fs_windows.go

// GetCacheSize returns total size of cached files
func (m *Manager) GetCacheSize() (int64, error) {
	var size int64
	err := filepath.Walk(m.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// CachePath returns the local cache path for a Synology file path
func (m *Manager) CachePath(synoPath string) string {
	return filepath.Join(m.rootDir, synoPath)
}

// EnsureDir ensures the directory for a file path exists
func (m *Manager) EnsureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return os.MkdirAll(dir, 0755)
}

// WriteFile writes content to the cache
func (m *Manager) WriteFile(synoPath string, reader io.Reader) (string, int64, error) {
	cachePath := m.CachePath(synoPath)

	// Ensure parent directory exists
	if err := m.EnsureDir(cachePath); err != nil {
		return "", 0, fmt.Errorf("failed to create parent dir: %w", err)
	}

	// Create temporary file first
	tmpPath := cachePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}

	// Write content
	written, err := io.Copy(f, reader)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to write file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to close file: %w", err)
	}

	// Rename to final path
	if err := os.Rename(tmpPath, cachePath); err != nil {
		os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to rename temp file: %w", err)
	}

	return cachePath, written, nil
}

// DeleteFile removes a cached file
func (m *Manager) DeleteFile(cachePath string) error {
	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// FileExists checks if a cached file exists
func (m *Manager) FileExists(cachePath string) bool {
	_, err := os.Stat(cachePath)
	return err == nil
}

// GetFileSize returns the size of a cached file
func (m *Manager) GetFileSize(cachePath string) (int64, error) {
	info, err := os.Stat(cachePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// CleanEmptyDirs removes empty directories under root
func (m *Manager) CleanEmptyDirs() error {
	return filepath.Walk(m.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && path != m.rootDir {
			// Try to remove - will only succeed if empty
			os.Remove(path)
		}
		return nil
	})
}
