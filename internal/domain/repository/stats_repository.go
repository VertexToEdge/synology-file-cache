package repository

import (
	"github.com/vertextoedge/synology-file-cache/internal/domain"
)

// StatsRepository defines the interface for cache statistics
type StatsRepository interface {
	// GetCacheStats returns cache statistics
	GetCacheStats() (*domain.CacheStats, error)
}
