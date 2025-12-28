package repository

import (
	"github.com/vertextoedge/synology-file-cache/internal/domain"
)

// FileRepository defines the interface for file persistence operations
type FileRepository interface {
	// GetByID retrieves a file by its internal ID
	GetByID(id int64) (*domain.File, error)

	// GetBySynoID retrieves a file by its Synology file ID
	GetBySynoID(synoID string) (*domain.File, error)

	// GetByPath retrieves a file by its path
	GetByPath(path string) (*domain.File, error)

	// Create creates a new file record
	Create(file *domain.File) error

	// Update updates an existing file record (including cache status)
	// Use UpdateMetadata for syncer to avoid overwriting cache status
	Update(file *domain.File) error

	// UpdateMetadata updates file metadata without touching cache status fields
	// This prevents race condition between syncer and cacher
	UpdateMetadata(file *domain.File) error

	// InvalidateCache sets cached=false for a file (used when source file is modified)
	InvalidateCache(fileID int64) error

	// Delete deletes a file record by ID
	Delete(id int64) error

	// GetEvictionCandidates returns cached files that can be evicted
	// Files are ordered by priority (lowest first) and then by LRU
	GetEvictionCandidates(limit int) ([]*domain.File, error)
}
