package syncer

import (
	"context"
	"fmt"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/scanner"
	"github.com/vertextoedge/synology-file-cache/internal/store"
	"github.com/vertextoedge/synology-file-cache/internal/synoapi"
	"go.uber.org/zap"
)

// Config contains syncer configuration
type Config struct {
	FullScanInterval    time.Duration
	IncrementalInterval time.Duration
	RecentModifiedDays  int
	RecentAccessedDays  int
	ExcludeLabels       []string // Labels to exclude from caching
}

// DriveSyncer synchronizes file metadata from Synology Drive
type DriveSyncer struct {
	config  *Config
	client  *synoapi.Client
	store   *store.Store
	logger  *zap.Logger
	scanner *scanner.Scanner
	running bool
	cancel  context.CancelFunc
}

// NewDriveSyncer creates a new Drive-based syncer
func NewDriveSyncer(cfg *Config, client *synoapi.Client, st *store.Store, logger *zap.Logger) *DriveSyncer {
	// Create scanner with limited concurrency
	scannerCfg := &scanner.Config{
		MaxConcurrency: 3, // Limit concurrent API calls
		BatchSize:      200,
	}
	sc := scanner.New(scannerCfg, client, st, logger)

	return &DriveSyncer{
		config:  cfg,
		client:  client,
		store:   st,
		logger:  logger,
		scanner: sc,
	}
}

// Start starts the sync loops
func (s *DriveSyncer) Start(ctx context.Context) error {
	if s.running {
		return fmt.Errorf("syncer already running")
	}
	s.running = true
	ctx, s.cancel = context.WithCancel(ctx)

	s.logger.Info("drive syncer started",
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
	s.logger.Info("drive syncer stopped")
	return nil
}

// Stop stops the syncer
func (s *DriveSyncer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
}

// fullScanLoop runs full scans periodically
func (s *DriveSyncer) fullScanLoop(ctx context.Context) {
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
func (s *DriveSyncer) incrementalLoop(ctx context.Context) {
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

// FullSync performs a full synchronization using Drive API
func (s *DriveSyncer) FullSync(ctx context.Context) error {
	s.logger.Info("starting Drive full sync")
	start := time.Now()

	// Sync shared files first (most important for cache server)
	sharedCount, err := s.syncSharedFiles(ctx)
	if err != nil {
		s.logger.Error("failed to sync shared files", zap.Error(err))
	}

	// Sync starred files
	starredCount, err := s.syncStarredFiles(ctx)
	if err != nil {
		s.logger.Error("failed to sync starred files", zap.Error(err))
	}

	// Sync labeled files
	labeledCount, err := s.syncLabeledFiles(ctx)
	if err != nil {
		s.logger.Error("failed to sync labeled files", zap.Error(err))
	}

	// Also get recent files for recently modified files
	recentCount, err := s.syncRecentFiles(ctx)
	if err != nil {
		s.logger.Error("failed to sync recent files", zap.Error(err))
	}

	s.logger.Info("Drive full sync completed",
		zap.Duration("duration", time.Since(start)),
		zap.Int("shared", sharedCount),
		zap.Int("starred", starredCount),
		zap.Int("labeled", labeledCount),
		zap.Int("recent", recentCount))

	return nil
}

// IncrementalSync performs an incremental sync
func (s *DriveSyncer) IncrementalSync(ctx context.Context) error {
	// Sync shared files on every incremental sync
	if _, err := s.syncSharedFiles(ctx); err != nil {
		s.logger.Warn("failed to sync shared files", zap.Error(err))
	}
	// Sync starred files
	if _, err := s.syncStarredFiles(ctx); err != nil {
		s.logger.Warn("failed to sync starred files", zap.Error(err))
	}
	// Sync labeled files
	if _, err := s.syncLabeledFiles(ctx); err != nil {
		s.logger.Warn("failed to sync labeled files", zap.Error(err))
	}
	_, err := s.syncRecentFiles(ctx)
	return err
}

// syncSharedFiles syncs files shared with others using shared_with_others API
func (s *DriveSyncer) syncSharedFiles(ctx context.Context) (int, error) {
	now := time.Now()
	count := 0
	offset := 0
	limit := 200 // Fetch in batches of 200

	for {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}

		// Get shared files with pagination
		shared, err := s.client.DriveGetSharedWithOthers(offset, limit)
		if err != nil {
			return count, fmt.Errorf("failed to get shared files at offset %d: %w", offset, err)
		}

		s.logger.Debug("fetched shared files batch",
			zap.Int("offset", offset),
			zap.Int("fetched", len(shared.Items)),
			zap.Int("total", shared.Total))

		if len(shared.Items) == 0 {
			break
		}

		for _, file := range shared.Items {
			select {
			case <-ctx.Done():
				return count, ctx.Err()
			default:
			}

			// Skip directories
			if file.IsDir() {
				continue
			}

			// Get file ID as string and int64
			fileID := file.ID.String()
			fileIDInt := file.GetID()

			// Check if file exists in database
			existing, err := s.store.GetFileBySynoID(fileID)
			if err != nil {
				s.logger.Warn("failed to check existing file",
					zap.String("path", file.Path),
					zap.Error(err))
				continue
			}

			if existing != nil {
				// Update existing file
				existing.Path = file.Path
				existing.Size = file.Size
				existing.Shared = true
				existing.LastSyncAt = &now

				// Shared files get highest priority
				if existing.Priority > store.PriorityShared {
					existing.Priority = store.PriorityShared
				}

				// Check if file was modified and invalidate cache if needed
				s.invalidateCacheIfModified(existing, file.MTime)

				if file.MTime > 0 {
					mtime := time.Unix(file.MTime, 0)
					existing.ModifiedAt = &mtime
				}

				if err := s.store.UpdateFile(existing); err != nil {
					s.logger.Warn("failed to update file",
						zap.String("path", file.Path),
						zap.Error(err))
					continue
				}

				// Create share record if file has permanent_link
				if file.PermanentLink != "" {
					s.createShareRecord(existing.ID, fileIDInt, file.PermanentLink)
				}
			} else {
				// Create new file
				newFile := &store.File{
					SynoFileID: fileID,
					Path:       file.Path,
					Size:       file.Size,
					Starred:    file.Starred,
					Shared:     true,
					Priority:   store.PriorityShared,
					LastSyncAt: &now,
				}

				if file.MTime > 0 {
					mtime := time.Unix(file.MTime, 0)
					newFile.ModifiedAt = &mtime
				}
				if file.ATime > 0 {
					atime := time.Unix(file.ATime, 0)
					newFile.AccessedAt = &atime
				}

				if err := s.store.CreateFile(newFile); err != nil {
					s.logger.Warn("failed to create file",
						zap.String("path", file.Path),
						zap.Error(err))
					continue
				}

				// Create share record if file has permanent_link
				if file.PermanentLink != "" {
					s.createShareRecord(newFile.ID, fileIDInt, file.PermanentLink)
				}

				s.logger.Debug("shared file added",
					zap.String("path", file.Path),
					zap.String("token", file.PermanentLink))
			}

			count++
		}

		// Move to next page
		offset += len(shared.Items)

		// Stop if we've fetched all items
		if offset >= shared.Total || len(shared.Items) < limit {
			break
		}
	}

	s.logger.Info("synced shared files", zap.Int("count", count))
	return count, nil
}

// syncStarredFiles syncs starred files using list_starred API
func (s *DriveSyncer) syncStarredFiles(ctx context.Context) (int, error) {
	now := time.Now()
	count := 0
	offset := 0
	limit := 200 // Fetch in batches of 200

	for {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}

		// Get starred files with pagination
		starred, err := s.client.DriveGetStarred(offset, limit)
		if err != nil {
			return count, fmt.Errorf("failed to get starred files at offset %d: %w", offset, err)
		}

		s.logger.Debug("fetched starred files batch",
			zap.Int("offset", offset),
			zap.Int("fetched", len(starred.Items)),
			zap.Int("total", starred.Total))

		if len(starred.Items) == 0 {
			break
		}

		for _, file := range starred.Items {
			select {
			case <-ctx.Done():
				return count, ctx.Err()
			default:
			}

			// For starred directories, scan contents recursively using scanner
			if file.IsDir() {
				result, err := s.scanner.ScanPath(ctx, file.Path, store.PriorityStarred)
				if err != nil {
					s.logger.Warn("failed to scan starred folder",
						zap.String("path", file.Path),
						zap.Error(err))
				} else {
					count += result.AddedFiles + result.UpdatedFiles
				}
				continue
			}

			// Get file ID as string
			fileID := file.ID.String()

			// Check if file exists in database
			existing, err := s.store.GetFileBySynoID(fileID)
			if err != nil {
				s.logger.Warn("failed to check existing file",
					zap.String("path", file.Path),
					zap.Error(err))
				continue
			}

			if existing != nil {
				// Update existing file
				existing.Path = file.Path
				existing.Size = file.Size
				existing.Starred = true
				existing.LastSyncAt = &now

				// Starred files get priority 2 (unless already shared with priority 1)
				if existing.Priority > store.PriorityStarred {
					existing.Priority = store.PriorityStarred
				}

				// Check if file was modified and invalidate cache if needed
				s.invalidateCacheIfModified(existing, file.MTime)

				if file.MTime > 0 {
					mtime := time.Unix(file.MTime, 0)
					existing.ModifiedAt = &mtime
				}

				if err := s.store.UpdateFile(existing); err != nil {
					s.logger.Warn("failed to update file",
						zap.String("path", file.Path),
						zap.Error(err))
					continue
				}
			} else {
				// Create new file
				newFile := &store.File{
					SynoFileID: fileID,
					Path:       file.Path,
					Size:       file.Size,
					Starred:    true,
					Shared:     file.Shared,
					Priority:   store.PriorityStarred,
					LastSyncAt: &now,
				}

				if file.MTime > 0 {
					mtime := time.Unix(file.MTime, 0)
					newFile.ModifiedAt = &mtime
				}
				if file.ATime > 0 {
					atime := time.Unix(file.ATime, 0)
					newFile.AccessedAt = &atime
				}

				if err := s.store.CreateFile(newFile); err != nil {
					s.logger.Warn("failed to create file",
						zap.String("path", file.Path),
						zap.Error(err))
					continue
				}

				s.logger.Debug("starred file added",
					zap.String("path", file.Path))
			}

			count++
		}

		// Move to next page
		offset += len(starred.Items)

		// Stop if we've fetched all items
		if offset >= starred.Total || len(starred.Items) < limit {
			break
		}
	}

	s.logger.Info("synced starred files", zap.Int("count", count))
	return count, nil
}

// syncLabeledFiles syncs files with labels
func (s *DriveSyncer) syncLabeledFiles(ctx context.Context) (int, error) {
	// First, get all labels
	labels, err := s.client.DriveGetLabels()
	if err != nil {
		return 0, fmt.Errorf("failed to get labels: %w", err)
	}

	s.logger.Info("found labels", zap.Int("count", len(labels)))

	if len(labels) == 0 {
		s.logger.Info("no labels found, skipping labeled files sync")
		return 0, nil
	}

	now := time.Now()
	totalCount := 0

	// For each label, get files with that label
	for _, label := range labels {
		// Check if this label should be excluded
		if s.isLabelExcluded(label.Name) {
			s.logger.Debug("skipping excluded label",
				zap.String("label_id", label.ID),
				zap.String("label_name", label.Name))
			continue
		}

		s.logger.Debug("syncing files with label",
			zap.String("label_id", label.ID),
			zap.String("label_name", label.Name))

		offset := 0
		limit := 200

		for {
			select {
			case <-ctx.Done():
				return totalCount, ctx.Err()
			default:
			}

			// Get files with this label
			files, err := s.client.DriveGetFilesByLabel(label.ID, offset, limit)
			if err != nil {
				s.logger.Warn("failed to get files for label",
					zap.String("label_id", label.ID),
					zap.String("label_name", label.Name),
					zap.Error(err))
				break
			}

			s.logger.Debug("fetched labeled files batch",
				zap.String("label", label.Name),
				zap.Int("offset", offset),
				zap.Int("fetched", len(files.Items)),
				zap.Int("total", files.Total))

			if len(files.Items) == 0 {
				break
			}

			for _, file := range files.Items {
				select {
				case <-ctx.Done():
					return totalCount, ctx.Err()
				default:
				}

				// For labeled directories, scan contents recursively
				if file.IsDir() {
					result, err := s.scanner.ScanPath(ctx, file.Path, store.PriorityStarred)
					if err != nil {
						s.logger.Warn("failed to scan labeled folder",
							zap.String("path", file.Path),
							zap.String("label", label.Name),
							zap.Error(err))
					} else {
						totalCount += result.AddedFiles + result.UpdatedFiles
					}
					continue
				}

				// Get file ID as string
				fileID := file.ID.String()

				// Check if file exists in database
				existing, err := s.store.GetFileBySynoID(fileID)
				if err != nil {
					s.logger.Warn("failed to check existing file",
						zap.String("path", file.Path),
						zap.Error(err))
					continue
				}

				if existing != nil {
					// Update existing file
					existing.Path = file.Path
					existing.Size = file.Size
					existing.LastSyncAt = &now

					// Labeled files get priority 2 (same as starred)
					if existing.Priority > store.PriorityStarred {
						existing.Priority = store.PriorityStarred
					}

					// Check if file was modified and invalidate cache if needed
					s.invalidateCacheIfModified(existing, file.MTime)

					if file.MTime > 0 {
						mtime := time.Unix(file.MTime, 0)
						existing.ModifiedAt = &mtime
					}

					if err := s.store.UpdateFile(existing); err != nil {
						s.logger.Warn("failed to update file",
							zap.String("path", file.Path),
							zap.Error(err))
						continue
					}
				} else {
					// Create new file
					newFile := &store.File{
						SynoFileID: fileID,
						Path:       file.Path,
						Size:       file.Size,
						Starred:    file.Starred,
						Shared:     file.Shared,
						Priority:   store.PriorityStarred, // Same priority as starred
						LastSyncAt: &now,
					}

					if file.MTime > 0 {
						mtime := time.Unix(file.MTime, 0)
						newFile.ModifiedAt = &mtime
					}
					if file.ATime > 0 {
						atime := time.Unix(file.ATime, 0)
						newFile.AccessedAt = &atime
					}

					if err := s.store.CreateFile(newFile); err != nil {
						s.logger.Warn("failed to create file",
							zap.String("path", file.Path),
							zap.Error(err))
						continue
					}

					s.logger.Debug("labeled file added",
						zap.String("path", file.Path),
						zap.String("label", label.Name))
				}

				totalCount++
			}

			// Move to next page
			offset += len(files.Items)

			// Stop if we've fetched all items
			if offset >= files.Total || len(files.Items) < limit {
				break
			}
		}
	}

	s.logger.Info("synced labeled files", zap.Int("count", totalCount))
	return totalCount, nil
}

// syncRecentFiles syncs recently modified files
func (s *DriveSyncer) syncRecentFiles(ctx context.Context) (int, error) {
	// Get recent files from Drive
	recent, err := s.client.DriveGetRecent(0, 200) // Get last 200 recent files
	if err != nil {
		return 0, fmt.Errorf("failed to get recent files: %w", err)
	}

	s.logger.Debug("fetched recent files",
		zap.Int("fetched", len(recent.Items)),
		zap.Int("total", recent.Total))

	now := time.Now()
	recentThreshold := now.AddDate(0, 0, -s.config.RecentModifiedDays)
	count := 0

	skippedDir := 0
	skippedSharedStarred := 0
	skippedOld := 0

	for _, file := range recent.Items {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}

		// Skip directories
		if file.IsDir() {
			skippedDir++
			continue
		}

		// Skip shared/starred files - they are handled by dedicated sync methods
		if file.Shared || file.Starred {
			skippedSharedStarred++
			continue
		}

		// Only process recently modified files
		if file.MTime <= 0 {
			skippedOld++
			continue
		}
		mtime := time.Unix(file.MTime, 0)
		if !mtime.After(recentThreshold) {
			skippedOld++
			continue
		}

		// Get file ID as string
		fileID := file.ID.String()

		// Check if file exists in database
		existing, err := s.store.GetFileBySynoID(fileID)
		if err != nil {
			s.logger.Warn("failed to check existing file",
				zap.String("path", file.Path),
				zap.Error(err))
			continue
		}

		if existing != nil {
			// Update existing file
			existing.Path = file.Path
			existing.Size = file.Size
			existing.LastSyncAt = &now

			// Only lower priority, don't raise it
			if store.PriorityRecentModified < existing.Priority {
				existing.Priority = store.PriorityRecentModified
			}

			// Check if file was modified and invalidate cache if needed
			s.invalidateCacheIfModified(existing, file.MTime)

			existing.ModifiedAt = &mtime

			if file.ATime > 0 {
				atime := time.Unix(file.ATime, 0)
				existing.AccessedAt = &atime
			}

			if err := s.store.UpdateFile(existing); err != nil {
				s.logger.Warn("failed to update file",
					zap.String("path", file.Path),
					zap.Error(err))
				continue
			}
		} else {
			// Create new file
			newFile := &store.File{
				SynoFileID: fileID,
				Path:       file.Path,
				Size:       file.Size,
				Starred:    false,
				Shared:     false,
				Priority:   store.PriorityRecentModified,
				LastSyncAt: &now,
				ModifiedAt: &mtime,
			}

			if file.ATime > 0 {
				atime := time.Unix(file.ATime, 0)
				newFile.AccessedAt = &atime
			}

			if err := s.store.CreateFile(newFile); err != nil {
				s.logger.Warn("failed to create file",
					zap.String("path", file.Path),
					zap.Error(err))
				continue
			}

			s.logger.Debug("recent file added",
				zap.String("path", file.Path))
		}

		count++
	}

	s.logger.Info("synced recent files",
		zap.Int("count", count),
		zap.Int("skipped_dir", skippedDir),
		zap.Int("skipped_shared_starred", skippedSharedStarred),
		zap.Int("skipped_old", skippedOld))
	return count, nil
}

// isLabelExcluded checks if a label name is in the exclude list
func (s *DriveSyncer) isLabelExcluded(labelName string) bool {
	for _, excluded := range s.config.ExcludeLabels {
		if excluded == labelName {
			return true
		}
	}
	return false
}

// invalidateCacheIfModified checks if file was modified and invalidates cache if needed
// Returns true if cache was invalidated
func (s *DriveSyncer) invalidateCacheIfModified(file *store.File, newMTime int64) bool {
	if !file.Cached {
		return false
	}

	if newMTime <= 0 {
		return false
	}

	newMtime := time.Unix(newMTime, 0)

	// If file has no recorded mtime, we can't compare - keep cache
	if file.ModifiedAt == nil {
		return false
	}

	// If new mtime is after cached mtime, invalidate cache
	if newMtime.After(*file.ModifiedAt) {
		s.logger.Info("file modified, invalidating cache",
			zap.String("path", file.Path),
			zap.Time("old_mtime", *file.ModifiedAt),
			zap.Time("new_mtime", newMtime))
		file.Cached = false
		file.CachePath.Valid = false
		file.CachePath.String = ""
		return true
	}

	return false
}

// createShareRecord creates a share record if it doesn't exist
func (s *DriveSyncer) createShareRecord(fileID int64, synoFileID int64, token string) {
	// Check if share already exists
	existingShare, err := s.store.GetShareByToken(token)
	if err != nil {
		s.logger.Warn("failed to check existing share",
			zap.String("token", token),
			zap.Error(err))
		return
	}

	if existingShare != nil {
		// Always update share info (password, expiration, sharing_link may have changed)
		s.updateShareWithAdvanceSharing(existingShare, synoFileID)
		return
	}

	// Get advanced sharing info to get sharing_link, url, password, and expiration
	var sharingLink, fullURL, password string
	var expiresAt *time.Time
	advInfo, err := s.client.DriveGetAdvanceSharing(synoFileID, "")
	if err != nil {
		s.logger.Warn("failed to get advance sharing info",
			zap.String("token", token),
			zap.Error(err))
	} else {
		sharingLink = advInfo.SharingLink
		fullURL = advInfo.URL
		password = advInfo.ProtectPassword
		if advInfo.DueDate > 0 {
			t := time.Unix(advInfo.DueDate, 0)
			expiresAt = &t
		}
	}

	// Create new share record
	newShare := &store.Share{
		SynoShareID: fmt.Sprintf("%d", synoFileID),
		Token:       token,
		SharingLink: sharingLink,
		URL:         fullURL,
		FileID:      fileID,
		ExpiresAt:   expiresAt,
		Revoked:     false,
	}
	if password != "" {
		newShare.Password.Valid = true
		newShare.Password.String = password
	}

	if err := s.store.CreateShare(newShare); err != nil {
		s.logger.Warn("failed to create share",
			zap.String("token", token),
			zap.Error(err))
		return
	}

	s.logger.Debug("share record created",
		zap.String("token", token),
		zap.String("sharing_link", sharingLink),
		zap.Int64("file_id", fileID))
}

// updateShareWithAdvanceSharing updates an existing share with AdvanceSharing info
func (s *DriveSyncer) updateShareWithAdvanceSharing(share *store.Share, synoFileID int64) {
	advInfo, err := s.client.DriveGetAdvanceSharing(synoFileID, "")
	if err != nil {
		s.logger.Warn("failed to get advance sharing info for update",
			zap.String("token", share.Token),
			zap.Error(err))
		return
	}

	share.SharingLink = advInfo.SharingLink
	share.URL = advInfo.URL

	// Update password
	if advInfo.ProtectPassword != "" {
		share.Password.Valid = true
		share.Password.String = advInfo.ProtectPassword
	} else {
		share.Password.Valid = false
		share.Password.String = ""
	}

	// Update expiration date
	if advInfo.DueDate > 0 {
		t := time.Unix(advInfo.DueDate, 0)
		share.ExpiresAt = &t
	} else {
		share.ExpiresAt = nil
	}

	if err := s.store.UpdateShare(share); err != nil {
		s.logger.Warn("failed to update share with sharing_link",
			zap.String("token", share.Token),
			zap.Error(err))
		return
	}

	s.logger.Debug("share record updated with sharing_link",
		zap.String("token", share.Token),
		zap.String("sharing_link", advInfo.SharingLink),
		zap.Bool("has_password", advInfo.ProtectPassword != ""))
}

// ScanFolder recursively scans a folder for files
func (s *DriveSyncer) ScanFolder(ctx context.Context, path string, depth int) error {
	if depth > 10 { // Prevent infinite recursion
		return nil
	}

	files, err := s.client.DriveListFiles(&synoapi.DriveListOptions{
		Path:  path,
		Limit: 100,
	})
	if err != nil {
		return fmt.Errorf("failed to list folder %s: %w", path, err)
	}

	now := time.Now()

	for _, file := range files.Items {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if file.IsDir() {
			// Recursively scan subdirectory
			if err := s.ScanFolder(ctx, file.Path, depth+1); err != nil {
				s.logger.Warn("failed to scan subfolder",
					zap.String("path", file.Path),
					zap.Error(err))
			}
			continue
		}

		// Skip non-priority files
		if !file.Shared && !file.Starred {
			continue
		}

		priority := store.PriorityDefault
		if file.Shared {
			priority = store.PriorityShared
		} else if file.Starred {
			priority = store.PriorityStarred
		}

		fileID := file.ID.String()
		existing, _ := s.store.GetFileBySynoID(fileID)

		if existing == nil {
			newFile := &store.File{
				SynoFileID: fileID,
				Path:       file.Path,
				Size:       file.Size,
				Starred:    file.Starred,
				Shared:     file.Shared,
				Priority:   priority,
				LastSyncAt: &now,
			}
			s.store.CreateFile(newFile)
		}
	}

	return nil
}
