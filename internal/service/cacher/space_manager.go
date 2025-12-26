package cacher

import (
	"github.com/vertextoedge/synology-file-cache/internal/port"
)

// SpaceManager handles space availability checks for caching
type SpaceManager struct {
	fs              port.FileSystem
	maxCacheSize    int64
	maxDiskUsagePct float64
}

// NewSpaceManager creates a new SpaceManager
func NewSpaceManager(fs port.FileSystem, maxCacheSize int64, maxDiskUsagePct float64) *SpaceManager {
	return &SpaceManager{
		fs:              fs,
		maxCacheSize:    maxCacheSize,
		maxDiskUsagePct: maxDiskUsagePct,
	}
}

// CheckSpace checks if there's enough space for a file of the given size
func (sm *SpaceManager) CheckSpace(fileSize int64) (*port.SpaceCheckResult, error) {
	result := &port.SpaceCheckResult{
		MaxCacheSizeBytes: sm.maxCacheSize,
		MaxDiskUsagePct:   sm.maxDiskUsagePct,
	}

	// Check cache size limit
	cacheSize, err := sm.fs.GetCacheSize()
	if err != nil {
		return nil, err
	}
	result.CacheSizeBytes = cacheSize
	result.AvailableBytes = sm.maxCacheSize - cacheSize

	if cacheSize+fileSize > sm.maxCacheSize {
		result.LimitedByCacheSize = true
		return result, nil
	}

	// Check disk usage limit
	usage, err := sm.fs.GetDiskUsage()
	if err != nil {
		return nil, err
	}
	result.DiskUsedPct = usage.UsedPct

	if usage.UsedPct >= sm.maxDiskUsagePct {
		result.LimitedByDiskUsage = true
		return result, nil
	}

	// Check if adding this file would exceed disk limit
	newUsedPct := float64(usage.Used+uint64(fileSize)) / float64(usage.Total) * 100
	if newUsedPct >= sm.maxDiskUsagePct {
		result.LimitedByDiskUsage = true
		return result, nil
	}

	result.HasSpace = true
	return result, nil
}

// HasSpace returns true if there's enough space for the given file size
func (sm *SpaceManager) HasSpace(fileSize int64) (bool, error) {
	result, err := sm.CheckSpace(fileSize)
	if err != nil {
		return false, err
	}
	return result.HasSpace, nil
}

// Ensure SpaceManager implements port.SpaceManager
var _ port.SpaceManager = (*SpaceManager)(nil)
