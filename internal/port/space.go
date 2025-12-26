package port

// SpaceCheckResult contains detailed space availability information
type SpaceCheckResult struct {
	HasSpace           bool
	AvailableBytes     int64
	CacheSizeBytes     int64
	MaxCacheSizeBytes  int64
	DiskUsedPct        float64
	MaxDiskUsagePct    float64
	LimitedByCacheSize bool
	LimitedByDiskUsage bool
}

// SpaceManager defines the interface for space management operations
type SpaceManager interface {
	// CheckSpace checks if there's enough space for a file of the given size
	// and returns detailed information about space availability
	CheckSpace(fileSize int64) (*SpaceCheckResult, error)

	// HasSpace returns true if there's enough space for the given file size
	HasSpace(fileSize int64) (bool, error)
}
