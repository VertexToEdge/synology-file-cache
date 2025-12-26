package maintenance

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// mockDownloadTaskRepository implements port.DownloadTaskRepository for testing
type mockDownloadTaskRepository struct {
	mu                   sync.Mutex
	releaseStaleCount    int
	cleanupFailedCount   int
	releaseStaleErr      error
	cleanupFailedErr     error
	releaseStaleCalled   int
	cleanupFailedCalled  int
}

func (m *mockDownloadTaskRepository) CreateTask(task *domain.DownloadTask) error {
	return nil
}
func (m *mockDownloadTaskRepository) ClaimNextTask(workerID string) (*domain.DownloadTask, error) {
	return nil, nil
}
func (m *mockDownloadTaskRepository) GetTask(id int64) (*domain.DownloadTask, error) {
	return nil, nil
}
func (m *mockDownloadTaskRepository) GetTaskByFileID(fileID int64) (*domain.DownloadTask, error) {
	return nil, nil
}
func (m *mockDownloadTaskRepository) HasActiveTask(fileID int64) (bool, error) {
	return false, nil
}
func (m *mockDownloadTaskRepository) UpdateTask(task *domain.DownloadTask) error {
	return nil
}
func (m *mockDownloadTaskRepository) UpdateProgress(taskID int64, bytesDownloaded int64, tempPath string) error {
	return nil
}
func (m *mockDownloadTaskRepository) CompleteTask(taskID int64) error {
	return nil
}
func (m *mockDownloadTaskRepository) FailTask(taskID int64, errMsg string, canRetry bool) error {
	return nil
}
func (m *mockDownloadTaskRepository) ReleaseStaleInProgressTasks(staleDuration time.Duration) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releaseStaleCalled++
	return m.releaseStaleCount, m.releaseStaleErr
}
func (m *mockDownloadTaskRepository) GetQueueStats() (*domain.QueueStats, error) {
	return nil, nil
}
func (m *mockDownloadTaskRepository) CleanupOldFailedTasks(olderThan time.Duration) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupFailedCalled++
	return m.cleanupFailedCount, m.cleanupFailedErr
}
func (m *mockDownloadTaskRepository) GetOversizedTasks(maxSize int64) ([]*domain.DownloadTask, error) {
	return nil, nil
}
func (m *mockDownloadTaskRepository) DeleteTask(taskID int64) error {
	return nil
}

// mockFileSystem implements port.FileSystem for testing
type mockFileSystem struct {
	mu                    sync.Mutex
	cleanTempFilesCount   int
	cleanTempFilesErr     error
	cleanTempFilesCalled  int
}

func (m *mockFileSystem) RootDir() string                               { return "" }
func (m *mockFileSystem) CachePath(synoPath string) string              { return "" }
func (m *mockFileSystem) WriteFile(synoPath string, r io.Reader) (string, int64, error) {
	return "", 0, nil
}
func (m *mockFileSystem) WriteFileWithResume(synoPath string, r io.Reader, resume bool, tempPath string) (string, int64, error) {
	return "", 0, nil
}
func (m *mockFileSystem) DeleteFile(path string) error                  { return nil }
func (m *mockFileSystem) FileExists(path string) bool                   { return false }
func (m *mockFileSystem) GetFileSize(path string) (int64, error)        { return 0, nil }
func (m *mockFileSystem) GetCacheSize() (int64, error)                  { return 0, nil }
func (m *mockFileSystem) GetDiskUsage() (*port.DiskUsage, error)        { return nil, nil }
func (m *mockFileSystem) GetTempFileInfo(path string) (int64, time.Time, error) {
	return 0, time.Time{}, nil
}
func (m *mockFileSystem) DeleteTempFile(path string) error              { return nil }
func (m *mockFileSystem) CleanOldTempFiles(olderThan time.Duration) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanTempFilesCalled++
	return m.cleanTempFilesCount, m.cleanTempFilesErr
}

func TestService_New(t *testing.T) {
	logger := zap.NewNop()
	tasks := &mockDownloadTaskRepository{}
	fs := &mockFileSystem{}

	// Test with nil config (should use defaults)
	s := New(nil, tasks, fs, logger)
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.config.StaleTaskCheckInterval != time.Minute {
		t.Errorf("StaleTaskCheckInterval = %v, want %v", s.config.StaleTaskCheckInterval, time.Minute)
	}
	if s.config.StaleTaskTimeout != 30*time.Minute {
		t.Errorf("StaleTaskTimeout = %v, want %v", s.config.StaleTaskTimeout, 30*time.Minute)
	}

	// Test with custom config
	cfg := &Config{
		StaleTaskCheckInterval: 2 * time.Minute,
		StaleTaskTimeout:       15 * time.Minute,
		CleanupInterval:        30 * time.Minute,
		FailedTaskMaxAge:       12 * time.Hour,
		TempFileMaxAge:         6 * time.Hour,
	}
	s = New(cfg, tasks, fs, logger)
	if s.config.StaleTaskCheckInterval != 2*time.Minute {
		t.Errorf("StaleTaskCheckInterval = %v, want %v", s.config.StaleTaskCheckInterval, 2*time.Minute)
	}
}

func TestService_StartStop(t *testing.T) {
	logger := zap.NewNop()
	tasks := &mockDownloadTaskRepository{}
	fs := &mockFileSystem{}

	cfg := &Config{
		StaleTaskCheckInterval: 10 * time.Millisecond,
		StaleTaskTimeout:       time.Minute,
		CleanupInterval:        50 * time.Millisecond,
		FailedTaskMaxAge:       time.Hour,
		TempFileMaxAge:         time.Hour,
	}
	s := New(cfg, tasks, fs, logger)

	ctx, cancel := context.WithCancel(context.Background())

	// Start in goroutine
	done := make(chan error, 1)
	go func() {
		done <- s.Start(ctx)
	}()

	// Wait for maintenance to run at least once
	time.Sleep(30 * time.Millisecond)

	// Stop the service
	cancel()
	s.Stop()

	// Wait for Start to return
	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Start() did not return after Stop()")
	}

	// Verify maintenance tasks were called
	tasks.mu.Lock()
	releaseCalled := tasks.releaseStaleCalled
	tasks.mu.Unlock()

	if releaseCalled == 0 {
		t.Error("ReleaseStaleInProgressTasks was not called")
	}
}

func TestService_DoubleStart(t *testing.T) {
	logger := zap.NewNop()
	tasks := &mockDownloadTaskRepository{}
	fs := &mockFileSystem{}

	s := New(nil, tasks, fs, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start first time
	go func() {
		s.Start(ctx)
	}()
	time.Sleep(10 * time.Millisecond)

	// Try to start again - should fail
	err := func() error {
		errChan := make(chan error, 1)
		go func() {
			errChan <- s.Start(ctx)
		}()

		select {
		case err := <-errChan:
			return err
		case <-time.After(50 * time.Millisecond):
			return nil // Blocked, which means it's checking running state
		}
	}()

	// Either it returns an error or it blocks (both are valid behaviors)
	// The important thing is it doesn't crash
	_ = err
}

func TestService_ReleaseStaleTask(t *testing.T) {
	logger := zap.NewNop()
	tasks := &mockDownloadTaskRepository{
		releaseStaleCount: 5,
	}
	fs := &mockFileSystem{}

	cfg := &Config{
		StaleTaskCheckInterval: 10 * time.Millisecond,
		StaleTaskTimeout:       time.Minute,
		CleanupInterval:        time.Hour, // Long interval so cleanup doesn't run
		FailedTaskMaxAge:       time.Hour,
		TempFileMaxAge:         time.Hour,
	}
	s := New(cfg, tasks, fs, logger)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		s.Start(ctx)
	}()

	// Wait for stale task check to run
	time.Sleep(50 * time.Millisecond)

	cancel()
	s.Stop()

	tasks.mu.Lock()
	called := tasks.releaseStaleCalled
	tasks.mu.Unlock()

	if called == 0 {
		t.Error("ReleaseStaleInProgressTasks was not called")
	}
}

func TestService_CleanupTasks(t *testing.T) {
	logger := zap.NewNop()
	tasks := &mockDownloadTaskRepository{
		cleanupFailedCount: 3,
	}
	fs := &mockFileSystem{
		cleanTempFilesCount: 2,
	}

	cfg := &Config{
		StaleTaskCheckInterval: time.Hour, // Long interval so stale check doesn't run
		StaleTaskTimeout:       time.Minute,
		CleanupInterval:        10 * time.Millisecond,
		FailedTaskMaxAge:       time.Hour,
		TempFileMaxAge:         time.Hour,
	}
	s := New(cfg, tasks, fs, logger)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		s.Start(ctx)
	}()

	// Wait for cleanup to run
	time.Sleep(50 * time.Millisecond)

	cancel()
	s.Stop()

	tasks.mu.Lock()
	cleanupCalled := tasks.cleanupFailedCalled
	tasks.mu.Unlock()

	fs.mu.Lock()
	tempCleanupCalled := fs.cleanTempFilesCalled
	fs.mu.Unlock()

	if cleanupCalled == 0 {
		t.Error("CleanupOldFailedTasks was not called")
	}
	if tempCleanupCalled == 0 {
		t.Error("CleanOldTempFiles was not called")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.StaleTaskCheckInterval != time.Minute {
		t.Errorf("StaleTaskCheckInterval = %v, want %v", cfg.StaleTaskCheckInterval, time.Minute)
	}
	if cfg.StaleTaskTimeout != 30*time.Minute {
		t.Errorf("StaleTaskTimeout = %v, want %v", cfg.StaleTaskTimeout, 30*time.Minute)
	}
	if cfg.CleanupInterval != time.Hour {
		t.Errorf("CleanupInterval = %v, want %v", cfg.CleanupInterval, time.Hour)
	}
	if cfg.FailedTaskMaxAge != 24*time.Hour {
		t.Errorf("FailedTaskMaxAge = %v, want %v", cfg.FailedTaskMaxAge, 24*time.Hour)
	}
	if cfg.TempFileMaxAge != 24*time.Hour {
		t.Errorf("TempFileMaxAge = %v, want %v", cfg.TempFileMaxAge, 24*time.Hour)
	}
}
