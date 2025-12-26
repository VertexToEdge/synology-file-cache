package syncer

import (
	"context"
	"fmt"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// Config contains syncer configuration
type Config struct {
	FullScanInterval    time.Duration
	IncrementalInterval time.Duration
	RecentModifiedDays  int
	RecentAccessedDays  int
	ExcludeLabels       []string
	ScanBatchSize       int
	ScanConcurrency     int
	PageSize            int
	MaxDownloadRetries  int
	MaxCacheSize        int64 // Maximum file size that can be cached
}

// DefaultConfig returns default syncer configuration
func DefaultConfig() *Config {
	return &Config{
		FullScanInterval:    time.Hour,
		IncrementalInterval: time.Minute,
		RecentModifiedDays:  30,
		RecentAccessedDays:  30,
		ScanBatchSize:       200,
		ScanConcurrency:     3,
		PageSize:            200,
		MaxDownloadRetries:  3,
	}
}

// Syncer synchronizes file metadata from Synology Drive
type Syncer struct {
	config      *Config
	drive       port.DriveClient
	files       port.FileRepository
	shares      port.ShareRepository
	tasks       port.DownloadTaskRepository
	logger      *zap.Logger
	scanner     *Scanner
	shareSyncer *ShareSyncer
	running     bool
	cancel      context.CancelFunc
}

// New creates a new Syncer
func New(cfg *Config, drive port.DriveClient, files port.FileRepository, shares port.ShareRepository, tasks port.DownloadTaskRepository, logger *zap.Logger) *Syncer {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	scanner := NewScanner(&ScannerConfig{
		MaxConcurrency: cfg.ScanConcurrency,
		BatchSize:      cfg.ScanBatchSize,
	}, drive, files, tasks, logger)

	shareSyncer := NewShareSyncer(drive, shares, logger)

	return &Syncer{
		config:      cfg,
		drive:       drive,
		files:       files,
		shares:      shares,
		tasks:       tasks,
		logger:      logger,
		scanner:     scanner,
		shareSyncer: shareSyncer,
	}
}

// Start starts the sync loops
func (s *Syncer) Start(ctx context.Context) error {
	if s.running {
		return fmt.Errorf("syncer already running")
	}
	s.running = true
	ctx, s.cancel = context.WithCancel(ctx)

	s.logger.Info("syncer started",
		zap.Duration("full_scan_interval", s.config.FullScanInterval),
		zap.Duration("incremental_interval", s.config.IncrementalInterval))

	// Run full scan immediately
	if err := s.FullSync(ctx); err != nil {
		s.logger.Error("initial full sync failed", zap.Error(err))
	}

	// Start background loops
	go s.fullScanLoop(ctx)
	go s.incrementalLoop(ctx)

	<-ctx.Done()
	s.logger.Info("syncer stopped")
	return nil
}

// Stop stops the syncer
func (s *Syncer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
}

// fullScanLoop runs full scans periodically
func (s *Syncer) fullScanLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.FullScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.FullSync(ctx); err != nil {
				s.logger.Error("full sync failed", zap.Error(err))
			}
		}
	}
}

// incrementalLoop runs incremental syncs periodically
func (s *Syncer) incrementalLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.IncrementalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.IncrementalSync(ctx); err != nil {
				s.logger.Error("incremental sync failed", zap.Error(err))
			}
		}
	}
}

// FullSync performs a full synchronization
func (s *Syncer) FullSync(ctx context.Context) error {
	s.logger.Info("starting full sync")
	start := time.Now()

	results := &SyncResults{}

	// Sync shared files (highest priority)
	count, err := s.syncSharedFiles(ctx)
	results.SharedCount = count
	if err != nil {
		s.logger.Error("failed to sync shared files", zap.Error(err))
	}

	// Sync starred files
	count, err = s.syncStarredFiles(ctx)
	results.StarredCount = count
	if err != nil {
		s.logger.Error("failed to sync starred files", zap.Error(err))
	}

	// Sync labeled files
	count, err = s.syncLabeledFiles(ctx)
	results.LabeledCount = count
	if err != nil {
		s.logger.Error("failed to sync labeled files", zap.Error(err))
	}

	// Sync recent files
	count, err = s.syncRecentFiles(ctx)
	results.RecentCount = count
	if err != nil {
		s.logger.Error("failed to sync recent files", zap.Error(err))
	}

	s.logger.Info("full sync completed",
		zap.Duration("duration", time.Since(start)),
		zap.Int("shared", results.SharedCount),
		zap.Int("starred", results.StarredCount),
		zap.Int("labeled", results.LabeledCount),
		zap.Int("recent", results.RecentCount))

	return nil
}

// IncrementalSync performs an incremental sync
func (s *Syncer) IncrementalSync(ctx context.Context) error {
	if _, err := s.syncSharedFiles(ctx); err != nil {
		s.logger.Warn("failed to sync shared files", zap.Error(err))
	}
	if _, err := s.syncStarredFiles(ctx); err != nil {
		s.logger.Warn("failed to sync starred files", zap.Error(err))
	}
	if _, err := s.syncLabeledFiles(ctx); err != nil {
		s.logger.Warn("failed to sync labeled files", zap.Error(err))
	}
	_, err := s.syncRecentFiles(ctx)
	return err
}

// SyncResults contains results from a sync operation
type SyncResults struct {
	SharedCount  int
	StarredCount int
	LabeledCount int
	RecentCount  int
}

// syncSharedFiles syncs files shared with others
func (s *Syncer) syncSharedFiles(ctx context.Context) (int, error) {
	opts := &SyncOptions{
		Priority:           domain.PriorityShared,
		UpdateShared:       true,
		CreateShareRecords: true,
	}

	count, err := s.syncFilesWithFetcher(ctx, s.drive.GetSharedFiles, opts)
	if err != nil {
		return count, err
	}

	s.logger.Info("synced shared files", zap.Int("count", count))
	return count, nil
}

// syncStarredFiles syncs starred files
func (s *Syncer) syncStarredFiles(ctx context.Context) (int, error) {
	opts := &SyncOptions{
		Priority:      domain.PriorityStarred,
		UpdateStarred: true,
		ScanDirs:      true,
	}

	count, err := s.syncFilesWithFetcher(ctx, s.drive.GetStarredFiles, opts)
	if err != nil {
		return count, err
	}

	s.logger.Info("synced starred files", zap.Int("count", count))
	return count, nil
}

// syncLabeledFiles syncs files with labels
func (s *Syncer) syncLabeledFiles(ctx context.Context) (int, error) {
	labels, err := s.drive.GetLabels()
	if err != nil {
		return 0, fmt.Errorf("failed to get labels: %w", err)
	}

	s.logger.Info("found labels", zap.Int("count", len(labels)))

	if len(labels) == 0 {
		return 0, nil
	}

	totalCount := 0

	for _, label := range labels {
		if s.isLabelExcluded(label.Name) {
			s.logger.Debug("skipping excluded label", zap.String("label", label.Name))
			continue
		}

		fetcher := func(offset, limit int) (*port.DriveListResponse, error) {
			return s.drive.GetLabeledFiles(label.ID, offset, limit)
		}

		opts := &SyncOptions{
			Priority: domain.PriorityStarred, // Same priority as starred
			ScanDirs: true,
		}

		count, err := s.syncFilesWithFetcher(ctx, fetcher, opts)
		if err != nil {
			s.logger.Warn("failed to sync files for label",
				zap.String("label", label.Name),
				zap.Error(err))
			continue
		}
		totalCount += count
	}

	s.logger.Info("synced labeled files", zap.Int("count", totalCount))
	return totalCount, nil
}

// syncRecentFiles syncs recently modified files
func (s *Syncer) syncRecentFiles(ctx context.Context) (int, error) {
	recent, err := s.drive.GetRecentFiles(0, 200)
	if err != nil {
		return 0, fmt.Errorf("failed to get recent files: %w", err)
	}

	s.logger.Debug("fetched recent files",
		zap.Int("fetched", len(recent.Items)),
		zap.Int("total", recent.Total))

	now := time.Now()
	recentThreshold := now.AddDate(0, 0, -s.config.RecentModifiedDays)
	count := 0

	for _, file := range recent.Items {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}

		// Skip directories
		if file.IsDir() {
			continue
		}

		// Skip shared/starred - handled by dedicated methods
		if file.Shared || file.Starred {
			continue
		}

		// Only process recently modified files
		mtime := file.GetMTime()
		if mtime == nil || !mtime.After(recentThreshold) {
			continue
		}

		if err := s.processFile(ctx, &file, domain.PriorityRecentModified, &now, nil); err != nil {
			s.logger.Warn("failed to process recent file",
				zap.String("path", file.Path),
				zap.Error(err))
			continue
		}
		count++
	}

	s.logger.Info("synced recent files", zap.Int("count", count))
	return count, nil
}

// isLabelExcluded checks if a label should be excluded
func (s *Syncer) isLabelExcluded(labelName string) bool {
	for _, excluded := range s.config.ExcludeLabels {
		if excluded == labelName {
			return true
		}
	}
	return false
}
