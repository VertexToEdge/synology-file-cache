//go:build windows
// +build windows

package fs

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpace = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// GetDiskUsage returns disk usage for the cache directory
func (m *Manager) GetDiskUsage() (*DiskUsage, error) {
	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64

	// Convert path to UTF16 pointer
	pathPtr, err := syscall.UTF16PtrFromString(m.rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to convert path: %w", err)
	}

	// Call GetDiskFreeSpaceExW
	ret, _, err := getDiskFreeSpace.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)

	if ret == 0 {
		return nil, fmt.Errorf("failed to get disk stats: %w", err)
	}

	used := totalNumberOfBytes - totalNumberOfFreeBytes

	return &DiskUsage{
		Total:   totalNumberOfBytes,
		Used:    used,
		Free:    totalNumberOfFreeBytes,
		UsedPct: float64(used) / float64(totalNumberOfBytes) * 100,
	}, nil
}
