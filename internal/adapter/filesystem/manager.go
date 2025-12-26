package filesystem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/port"
)

// Manager handles local filesystem operations
type Manager struct {
	rootDir    string
	bufferSize int
}

// Ensure Manager implements port.FileSystem
var _ port.FileSystem = (*Manager)(nil)

// NewManager creates a new filesystem manager
func NewManager(rootDir string) (*Manager, error) {
	return NewManagerWithBufferSize(rootDir, 8*1024*1024) // 8MB default
}

// NewManagerWithBufferSize creates a new filesystem manager with custom buffer size
func NewManagerWithBufferSize(rootDir string, bufferSize int) (*Manager, error) {
	// Ensure root directory exists
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache root dir: %w", err)
	}

	if bufferSize <= 0 {
		bufferSize = 8 * 1024 * 1024 // 8MB default
	}

	return &Manager{
		rootDir:    rootDir,
		bufferSize: bufferSize,
	}, nil
}

// RootDir returns the cache root directory
func (m *Manager) RootDir() string {
	return m.rootDir
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
	return m.WriteFileWithResume(synoPath, reader, false, "")
}

// WriteFileWithResume writes content with optional resume support
func (m *Manager) WriteFileWithResume(synoPath string, reader io.Reader, resume bool, tempPath string) (string, int64, error) {
	cachePath := m.CachePath(synoPath)

	// Ensure parent directory exists
	if err := m.EnsureDir(cachePath); err != nil {
		return "", 0, fmt.Errorf("failed to create parent dir: %w", err)
	}

	// If tempPath is not provided, generate a default one
	if tempPath == "" {
		tempPath = cachePath + ".downloading"
	}

	var f *os.File
	var existingSize int64
	var err error

	if resume {
		// Check if temp file exists and get its size
		if info, statErr := os.Stat(tempPath); statErr == nil {
			existingSize = info.Size()
			// Open in append mode
			f, err = os.OpenFile(tempPath, os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return "", 0, fmt.Errorf("failed to open temp file for resume: %w", err)
			}
		} else {
			// Temp file doesn't exist, create new
			f, err = os.Create(tempPath)
			if err != nil {
				return "", 0, fmt.Errorf("failed to create temp file: %w", err)
			}
		}
	} else {
		// Always create new temp file
		f, err = os.Create(tempPath)
		if err != nil {
			return "", 0, fmt.Errorf("failed to create temp file: %w", err)
		}
	}

	// Use configurable buffer for better performance on high-speed networks
	buf := make([]byte, m.bufferSize)
	written, err := io.CopyBuffer(f, reader, buf)
	if err != nil {
		f.Close()
		return "", 0, fmt.Errorf("failed to write file: %w", err)
	}

	if err := f.Close(); err != nil {
		return "", 0, fmt.Errorf("failed to close file: %w", err)
	}

	// Calculate total written
	totalWritten := existingSize + written

	// Rename to final path
	if err := os.Rename(tempPath, cachePath); err != nil {
		return "", 0, fmt.Errorf("failed to rename temp file: %w", err)
	}

	return cachePath, totalWritten, nil
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

// GetTempFileInfo returns size and modification time of a temp file
// Returns error if file does not exist
func (m *Manager) GetTempFileInfo(tempPath string) (int64, time.Time, error) {
	info, err := os.Stat(tempPath)
	if err != nil {
		return 0, time.Time{}, err // Return error for non-existent files too
	}
	return info.Size(), info.ModTime(), nil
}

// DeleteTempFile removes a temporary file
func (m *Manager) DeleteTempFile(tempPath string) error {
	if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete temp file: %w", err)
	}
	return nil
}

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

// CleanOldTempFiles removes temp files older than the specified duration
func (m *Manager) CleanOldTempFiles(olderThan time.Duration) (int, error) {
	count := 0
	threshold := time.Now().Add(-olderThan)

	err := filepath.Walk(m.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			ext := filepath.Ext(path)
			if ext == ".downloading" {
				if info.ModTime().Before(threshold) {
					if removeErr := os.Remove(path); removeErr == nil {
						count++
					}
				}
			}
		}
		return nil
	})
	return count, err
}

// CleanEmptyDirs removes empty directories under root
func (m *Manager) CleanEmptyDirs() error {
	return filepath.Walk(m.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && path != m.rootDir {
			os.Remove(path) // Will only succeed if empty
		}
		return nil
	})
}
