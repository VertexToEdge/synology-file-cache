package cacher

import (
	"io"
	"testing"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/port"
)

// mockFileSystem implements port.FileSystem for testing
type mockFileSystem struct {
	cacheSize int64
	diskUsage *port.DiskUsage
	err       error
}

func (m *mockFileSystem) GetCacheSize() (int64, error) {
	return m.cacheSize, m.err
}

func (m *mockFileSystem) GetDiskUsage() (*port.DiskUsage, error) {
	return m.diskUsage, m.err
}

// Stub implementations for other FileSystem methods
func (m *mockFileSystem) RootDir() string                                                          { return "" }
func (m *mockFileSystem) CachePath(synoPath string) string                                         { return "" }
func (m *mockFileSystem) WriteFile(synoPath string, r io.Reader) (string, int64, error)            { return "", 0, nil }
func (m *mockFileSystem) WriteFileWithResume(synoPath string, r io.Reader, resume bool, tempPath string) (string, int64, error) {
	return "", 0, nil
}
func (m *mockFileSystem) DeleteFile(path string) error                                             { return nil }
func (m *mockFileSystem) FileExists(path string) bool                                              { return false }
func (m *mockFileSystem) GetFileSize(path string) (int64, error)                                   { return 0, nil }
func (m *mockFileSystem) GetTempFileInfo(path string) (int64, time.Time, error)                    { return 0, time.Time{}, nil }
func (m *mockFileSystem) DeleteTempFile(path string) error                                         { return nil }
func (m *mockFileSystem) CleanOldTempFiles(olderThan time.Duration) (int, error)                   { return 0, nil }

func TestSpaceManager_CheckSpace(t *testing.T) {
	tests := []struct {
		name              string
		maxCacheSize      int64
		maxDiskUsagePct   float64
		cacheSize         int64
		diskUsage         *port.DiskUsage
		fileSize          int64
		wantHasSpace      bool
		wantLimitedCache  bool
		wantLimitedDisk   bool
	}{
		{
			name:            "has space - well under limits",
			maxCacheSize:    100 * 1024 * 1024 * 1024, // 100GB
			maxDiskUsagePct: 80,
			cacheSize:       10 * 1024 * 1024 * 1024, // 10GB
			diskUsage: &port.DiskUsage{
				Total:   1000 * 1024 * 1024 * 1024, // 1TB
				Used:    400 * 1024 * 1024 * 1024,  // 400GB (40%)
				Free:    600 * 1024 * 1024 * 1024,
				UsedPct: 40,
			},
			fileSize:         1 * 1024 * 1024 * 1024, // 1GB
			wantHasSpace:     true,
			wantLimitedCache: false,
			wantLimitedDisk:  false,
		},
		{
			name:            "limited by cache size",
			maxCacheSize:    50 * 1024 * 1024 * 1024, // 50GB
			maxDiskUsagePct: 80,
			cacheSize:       49 * 1024 * 1024 * 1024, // 49GB
			diskUsage: &port.DiskUsage{
				Total:   1000 * 1024 * 1024 * 1024,
				Used:    400 * 1024 * 1024 * 1024,
				Free:    600 * 1024 * 1024 * 1024,
				UsedPct: 40,
			},
			fileSize:         2 * 1024 * 1024 * 1024, // 2GB - exceeds limit
			wantHasSpace:     false,
			wantLimitedCache: true,
			wantLimitedDisk:  false,
		},
		{
			name:            "limited by current disk usage",
			maxCacheSize:    100 * 1024 * 1024 * 1024,
			maxDiskUsagePct: 50,
			cacheSize:       10 * 1024 * 1024 * 1024,
			diskUsage: &port.DiskUsage{
				Total:   1000 * 1024 * 1024 * 1024,
				Used:    500 * 1024 * 1024 * 1024, // 50% - at limit
				Free:    500 * 1024 * 1024 * 1024,
				UsedPct: 50,
			},
			fileSize:         1 * 1024 * 1024 * 1024,
			wantHasSpace:     false,
			wantLimitedCache: false,
			wantLimitedDisk:  true,
		},
		{
			name:            "limited by projected disk usage",
			maxCacheSize:    100 * 1024 * 1024 * 1024,
			maxDiskUsagePct: 50,
			cacheSize:       10 * 1024 * 1024 * 1024,
			diskUsage: &port.DiskUsage{
				Total:   1000 * 1024 * 1024 * 1024,
				Used:    450 * 1024 * 1024 * 1024, // 45%
				Free:    550 * 1024 * 1024 * 1024,
				UsedPct: 45,
			},
			fileSize:         60 * 1024 * 1024 * 1024, // Would push to 51%
			wantHasSpace:     false,
			wantLimitedCache: false,
			wantLimitedDisk:  true,
		},
		{
			name:            "exactly at cache limit - still ok",
			maxCacheSize:    50 * 1024 * 1024 * 1024,
			maxDiskUsagePct: 80,
			cacheSize:       49 * 1024 * 1024 * 1024,
			diskUsage: &port.DiskUsage{
				Total:   1000 * 1024 * 1024 * 1024,
				Used:    400 * 1024 * 1024 * 1024,
				Free:    600 * 1024 * 1024 * 1024,
				UsedPct: 40,
			},
			fileSize:         1 * 1024 * 1024 * 1024, // Exactly at limit (49+1=50)
			wantHasSpace:     true,
			wantLimitedCache: false,
			wantLimitedDisk:  false,
		},
		{
			name:            "zero file size",
			maxCacheSize:    50 * 1024 * 1024 * 1024,
			maxDiskUsagePct: 80,
			cacheSize:       50 * 1024 * 1024 * 1024, // At limit
			diskUsage: &port.DiskUsage{
				Total:   1000 * 1024 * 1024 * 1024,
				Used:    800 * 1024 * 1024 * 1024, // At limit
				Free:    200 * 1024 * 1024 * 1024,
				UsedPct: 80,
			},
			fileSize:         0, // Zero size file
			wantHasSpace:     false,
			wantLimitedCache: false,
			wantLimitedDisk:  true, // Disk at limit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &mockFileSystem{
				cacheSize: tt.cacheSize,
				diskUsage: tt.diskUsage,
			}

			sm := NewSpaceManager(fs, tt.maxCacheSize, tt.maxDiskUsagePct)
			result, err := sm.CheckSpace(tt.fileSize)

			if err != nil {
				t.Fatalf("CheckSpace() error = %v", err)
			}

			if result.HasSpace != tt.wantHasSpace {
				t.Errorf("HasSpace = %v, want %v", result.HasSpace, tt.wantHasSpace)
			}

			if result.LimitedByCacheSize != tt.wantLimitedCache {
				t.Errorf("LimitedByCacheSize = %v, want %v", result.LimitedByCacheSize, tt.wantLimitedCache)
			}

			if result.LimitedByDiskUsage != tt.wantLimitedDisk {
				t.Errorf("LimitedByDiskUsage = %v, want %v", result.LimitedByDiskUsage, tt.wantLimitedDisk)
			}

			// Check that result contains correct config values
			if result.MaxCacheSizeBytes != tt.maxCacheSize {
				t.Errorf("MaxCacheSizeBytes = %v, want %v", result.MaxCacheSizeBytes, tt.maxCacheSize)
			}

			if result.MaxDiskUsagePct != tt.maxDiskUsagePct {
				t.Errorf("MaxDiskUsagePct = %v, want %v", result.MaxDiskUsagePct, tt.maxDiskUsagePct)
			}
		})
	}
}

func TestSpaceManager_HasSpace(t *testing.T) {
	tests := []struct {
		name      string
		cacheSize int64
		diskUsage *port.DiskUsage
		fileSize  int64
		want      bool
	}{
		{
			name:      "has space",
			cacheSize: 10 * 1024 * 1024 * 1024,
			diskUsage: &port.DiskUsage{
				Total:   1000 * 1024 * 1024 * 1024,
				Used:    400 * 1024 * 1024 * 1024,
				Free:    600 * 1024 * 1024 * 1024,
				UsedPct: 40,
			},
			fileSize: 1 * 1024 * 1024 * 1024,
			want:     true,
		},
		{
			name:      "no space",
			cacheSize: 49 * 1024 * 1024 * 1024,
			diskUsage: &port.DiskUsage{
				Total:   1000 * 1024 * 1024 * 1024,
				Used:    400 * 1024 * 1024 * 1024,
				Free:    600 * 1024 * 1024 * 1024,
				UsedPct: 40,
			},
			fileSize: 2 * 1024 * 1024 * 1024,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &mockFileSystem{
				cacheSize: tt.cacheSize,
				diskUsage: tt.diskUsage,
			}

			sm := NewSpaceManager(fs, 50*1024*1024*1024, 80)
			got, err := sm.HasSpace(tt.fileSize)

			if err != nil {
				t.Fatalf("HasSpace() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("HasSpace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpaceManager_AvailableBytes(t *testing.T) {
	fs := &mockFileSystem{
		cacheSize: 30 * 1024 * 1024 * 1024, // 30GB used
		diskUsage: &port.DiskUsage{
			Total:   1000 * 1024 * 1024 * 1024,
			Used:    400 * 1024 * 1024 * 1024,
			Free:    600 * 1024 * 1024 * 1024,
			UsedPct: 40,
		},
	}

	maxCacheSize := int64(50 * 1024 * 1024 * 1024) // 50GB max
	sm := NewSpaceManager(fs, maxCacheSize, 80)

	result, err := sm.CheckSpace(1)
	if err != nil {
		t.Fatalf("CheckSpace() error = %v", err)
	}

	expectedAvailable := maxCacheSize - (30 * 1024 * 1024 * 1024) // 20GB
	if result.AvailableBytes != expectedAvailable {
		t.Errorf("AvailableBytes = %v, want %v", result.AvailableBytes, expectedAvailable)
	}
}
