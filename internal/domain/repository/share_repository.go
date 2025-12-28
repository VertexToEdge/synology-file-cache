package repository

import (
	"github.com/vertextoedge/synology-file-cache/internal/domain"
)

// ShareRepository defines the interface for share persistence operations
type ShareRepository interface {
	// GetShareByToken retrieves a share by its token
	GetShareByToken(token string) (*domain.Share, error)

	// GetFileByShareToken retrieves both the file and share by share token
	GetFileByShareToken(token string) (*domain.File, *domain.Share, error)

	// CreateShare creates a new share record
	CreateShare(share *domain.Share) error

	// UpdateShare updates an existing share record
	UpdateShare(share *domain.Share) error
}
