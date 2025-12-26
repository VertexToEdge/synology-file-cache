//go:build !windows
// +build !windows

package filesystem

import (
	"fmt"
	"syscall"

	"github.com/vertextoedge/synology-file-cache/internal/port"
)

// GetDiskUsage returns disk usage for the cache directory
func (m *Manager) GetDiskUsage() (*port.DiskUsage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(m.rootDir, &stat); err != nil {
		return nil, fmt.Errorf("failed to get disk stats: %w", err)
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := total - free

	return &port.DiskUsage{
		Total:   total,
		Used:    used,
		Free:    free,
		UsedPct: float64(used) / float64(total) * 100,
	}, nil
}
