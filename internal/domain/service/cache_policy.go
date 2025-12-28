package service

import (
	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/domain/vo"
)

// CachePolicy is a domain service that manages caching policies
type CachePolicy struct {
	maxFileSize     vo.FileSize
	maxCacheSize    vo.FileSize
	maxDiskUsagePct float64
}

// NewCachePolicy creates a new CachePolicy
func NewCachePolicy(maxFileSizeGB, maxCacheSizeGB float64, maxDiskUsagePct float64) *CachePolicy {
	return &CachePolicy{
		maxFileSize:     vo.FileSizeFromGB(maxFileSizeGB),
		maxCacheSize:    vo.FileSizeFromGB(maxCacheSizeGB),
		maxDiskUsagePct: maxDiskUsagePct,
	}
}

// NewCachePolicyFromBytes creates a CachePolicy from byte values
func NewCachePolicyFromBytes(maxFileSize, maxCacheSize int64, maxDiskUsagePct float64) *CachePolicy {
	return &CachePolicy{
		maxFileSize:     vo.MustFileSize(maxFileSize),
		maxCacheSize:    vo.MustFileSize(maxCacheSize),
		maxDiskUsagePct: maxDiskUsagePct,
	}
}

// SpaceCheckResult contains the result of a space check
type SpaceCheckResult struct {
	HasSpace           bool
	AvailableSpace     vo.FileSize
	LimitedByCacheSize bool
	LimitedByDiskUsage bool
	CurrentCacheSize   vo.FileSize
	CurrentDiskUsage   float64
}

// CanCache checks if a file can be cached based on size limits
func (cp *CachePolicy) CanCache(file *domain.File) error {
	return file.CanBeCached(cp.maxFileSize)
}

// CanCacheSize checks if a file of given size can be cached
func (cp *CachePolicy) CanCacheSize(size vo.FileSize) error {
	if size.ExceedsLimit(cp.maxFileSize) {
		return domain.ErrFileTooLarge
	}
	return nil
}

// CheckSpace checks if there's enough space to cache a file
func (cp *CachePolicy) CheckSpace(fileSize, currentCacheSize vo.FileSize, currentDiskUsagePct float64) SpaceCheckResult {
	result := SpaceCheckResult{
		CurrentCacheSize: currentCacheSize,
		CurrentDiskUsage: currentDiskUsagePct,
	}

	// Check cache size limit
	projectedCacheSize := currentCacheSize.Add(fileSize)
	if projectedCacheSize.ExceedsLimit(cp.maxCacheSize) {
		result.LimitedByCacheSize = true
		return result
	}

	// Check disk usage limit
	if currentDiskUsagePct >= cp.maxDiskUsagePct {
		result.LimitedByDiskUsage = true
		return result
	}

	result.HasSpace = true
	result.AvailableSpace = cp.maxCacheSize.Subtract(currentCacheSize)
	return result
}

// GetMaxFileSize returns the maximum file size
func (cp *CachePolicy) GetMaxFileSize() vo.FileSize {
	return cp.maxFileSize
}

// GetMaxCacheSize returns the maximum cache size
func (cp *CachePolicy) GetMaxCacheSize() vo.FileSize {
	return cp.maxCacheSize
}

// GetMaxDiskUsagePct returns the maximum disk usage percentage
func (cp *CachePolicy) GetMaxDiskUsagePct() float64 {
	return cp.maxDiskUsagePct
}

// ExceedsMaxFileSize checks if a file size exceeds the maximum
func (cp *CachePolicy) ExceedsMaxFileSize(size int64) bool {
	return vo.MustFileSize(size).ExceedsLimit(cp.maxFileSize)
}

// ShouldEvict determines if eviction is needed
func (cp *CachePolicy) ShouldEvict(neededSpace, currentCacheSize vo.FileSize, currentDiskUsagePct float64) bool {
	// Check if we need to evict based on cache size
	if currentCacheSize.Add(neededSpace).ExceedsLimit(cp.maxCacheSize) {
		return true
	}

	// Check if we need to evict based on disk usage
	if currentDiskUsagePct >= cp.maxDiskUsagePct {
		return true
	}

	return false
}
