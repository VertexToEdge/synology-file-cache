package service

import (
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/domain/vo"
)

// PriorityCalculator is a domain service that calculates file priority
type PriorityCalculator struct {
	recentModifiedThreshold time.Duration
	recentAccessedThreshold time.Duration
}

// NewPriorityCalculator creates a new PriorityCalculator
func NewPriorityCalculator(recentModifiedDays, recentAccessedDays int) *PriorityCalculator {
	return &PriorityCalculator{
		recentModifiedThreshold: time.Duration(recentModifiedDays) * 24 * time.Hour,
		recentAccessedThreshold: time.Duration(recentAccessedDays) * 24 * time.Hour,
	}
}

// DefaultPriorityCalculator creates a calculator with default thresholds
func DefaultPriorityCalculator() *PriorityCalculator {
	return NewPriorityCalculator(30, 7)
}

// Calculate determines the priority for a file based on its attributes
func (pc *PriorityCalculator) Calculate(file *domain.File) vo.Priority {
	// Shared files have highest priority
	if file.IsShared() {
		return vo.PriorityShared
	}

	// Starred files have second priority
	if file.IsStarred() {
		return vo.PriorityStarred
	}

	// Recently modified files
	if file.IsRecentlyModified(pc.recentModifiedThreshold) {
		return vo.PriorityRecentModified
	}

	// Recently accessed files
	if file.IsRecentlyAccessed(pc.recentAccessedThreshold) {
		return vo.PriorityRecentAccessed
	}

	// Default priority
	return vo.PriorityDefault
}

// CalculateForCategory returns the priority for a specific sync category
func (pc *PriorityCalculator) CalculateForCategory(category FileCategory) vo.Priority {
	switch category {
	case CategoryShared:
		return vo.PriorityShared
	case CategoryStarred, CategoryLabeled:
		return vo.PriorityStarred
	case CategoryRecentModified:
		return vo.PriorityRecentModified
	case CategoryRecentAccessed:
		return vo.PriorityRecentAccessed
	default:
		return vo.PriorityDefault
	}
}

// FileCategory represents a category of files for sync
type FileCategory string

const (
	CategoryShared         FileCategory = "shared"
	CategoryStarred        FileCategory = "starred"
	CategoryLabeled        FileCategory = "labeled"
	CategoryRecentModified FileCategory = "recent_modified"
	CategoryRecentAccessed FileCategory = "recent_accessed"
	CategoryDefault        FileCategory = "default"
)

// UpdateFilePriority updates the file priority if the new priority is higher
func (pc *PriorityCalculator) UpdateFilePriority(file *domain.File, category FileCategory) bool {
	newPriority := pc.CalculateForCategory(category)
	return file.UpdatePriorityVO(newPriority)
}
