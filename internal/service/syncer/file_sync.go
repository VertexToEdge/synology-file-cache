package syncer

import (
	"context"
	"fmt"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// FileFetcher is a function that fetches files with pagination
type FileFetcher func(offset, limit int) (*port.DriveListResponse, error)

// SyncOptions contains options for syncing files
type SyncOptions struct {
	Priority           int
	UpdateShared       bool
	UpdateStarred      bool
	CreateShareRecords bool
	ScanDirs           bool
}

// syncFilesWithFetcher syncs files using a generic fetcher function
// This eliminates code duplication between syncSharedFiles, syncStarredFiles, etc.
func (s *Syncer) syncFilesWithFetcher(ctx context.Context, fetcher FileFetcher, opts *SyncOptions) (int, error) {
	now := time.Now()
	count := 0
	offset := 0
	limit := 200

	for {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}

		// Fetch files with pagination
		resp, err := fetcher(offset, limit)
		if err != nil {
			return count, fmt.Errorf("failed to fetch files at offset %d: %w", offset, err)
		}

		s.logger.Debug("fetched files batch",
			zap.Int("offset", offset),
			zap.Int("fetched", len(resp.Items)),
			zap.Int("total", resp.Total))

		if len(resp.Items) == 0 {
			break
		}

		for _, file := range resp.Items {
			select {
			case <-ctx.Done():
				return count, ctx.Err()
			default:
			}

			// Handle directories with scanning
			if file.IsDir() {
				if opts.ScanDirs {
					result, err := s.scanner.ScanPath(ctx, file.Path, opts.Priority)
					if err != nil {
						s.logger.Warn("failed to scan folder",
							zap.String("path", file.Path),
							zap.Error(err))
					} else {
						count += result.AddedFiles + result.UpdatedFiles
					}
				}
				continue
			}

			if err := s.processFile(ctx, &file, opts.Priority, &now, opts); err != nil {
				s.logger.Warn("failed to process file",
					zap.String("path", file.Path),
					zap.Error(err))
				continue
			}
			count++
		}

		// Move to next page
		offset += len(resp.Items)

		// Stop if we've fetched all items
		if offset >= resp.Total || len(resp.Items) < limit {
			break
		}
	}

	return count, nil
}

// processFile creates or updates a file in the database and enqueues download task if needed
func (s *Syncer) processFile(ctx context.Context, file *port.DriveFile, priority int, now *time.Time, opts *SyncOptions) error {
	fileID := file.GetIDString()
	fileIDInt := file.GetID()

	// Check if file exists in database
	existing, err := s.files.GetBySynoID(fileID)
	if err != nil {
		return fmt.Errorf("failed to check existing file: %w", err)
	}

	var dbFile *domain.File
	var wasInvalidated bool
	var isNew bool

	if existing != nil {
		dbFile = existing

		// Update existing file metadata
		existing.Path = file.Path
		existing.Size = file.Size
		existing.LastSyncAt = now

		// Update flags based on options
		if opts != nil {
			if opts.UpdateShared {
				existing.Shared = true
			}
			if opts.UpdateStarred {
				existing.Starred = true
			}
		}

		// Update priority if higher
		existing.UpdatePriority(priority)

		// Check if file was modified and invalidate cache
		mtime := file.GetMTime()
		if mtime != nil && existing.ShouldInvalidateCache(*mtime) {
			s.logger.Info("file modified, invalidating cache",
				zap.String("path", file.Path),
				zap.Time("old_mtime", *existing.ModifiedAt),
				zap.Time("new_mtime", *mtime))
			// Use dedicated InvalidateCache to avoid race condition
			if err := s.files.InvalidateCache(existing.ID); err != nil {
				s.logger.Warn("failed to invalidate cache",
					zap.String("path", file.Path),
					zap.Error(err))
			}
			existing.Cached = false
			existing.CachePath = ""
			wasInvalidated = true
		}

		if mtime != nil {
			existing.ModifiedAt = mtime
		}
		if atime := file.GetATime(); atime != nil {
			existing.AccessedAt = atime
		}

		// Use UpdateMetadata to avoid overwriting cache status set by cacher
		if err := s.files.UpdateMetadata(existing); err != nil {
			return fmt.Errorf("failed to update file: %w", err)
		}

		// Create share record if needed
		if opts != nil && opts.CreateShareRecords && file.PermanentLink != "" {
			s.createOrUpdateShareRecord(existing.ID, fileIDInt, file.PermanentLink)
		}
	} else {
		isNew = true

		// Create new file
		newFile := &domain.File{
			SynoFileID: fileID,
			Path:       file.Path,
			Size:       file.Size,
			Starred:    file.Starred,
			Shared:     file.Shared,
			Priority:   priority,
			LastSyncAt: now,
		}

		// Update flags based on options
		if opts != nil {
			if opts.UpdateShared {
				newFile.Shared = true
			}
			if opts.UpdateStarred {
				newFile.Starred = true
			}
		}

		if mtime := file.GetMTime(); mtime != nil {
			newFile.ModifiedAt = mtime
		}
		if atime := file.GetATime(); atime != nil {
			newFile.AccessedAt = atime
		}

		if err := s.files.Create(newFile); err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}

		dbFile = newFile

		// Create share record if needed
		if opts != nil && opts.CreateShareRecords && file.PermanentLink != "" {
			s.createOrUpdateShareRecord(newFile.ID, fileIDInt, file.PermanentLink)
		}

		s.logger.Debug("file added",
			zap.String("path", file.Path),
			zap.Int("priority", priority))
	}

	// Enqueue download task if file needs caching
	if isNew || wasInvalidated || !dbFile.Cached {
		s.enqueueDownloadTask(dbFile)
	}

	return nil
}

// enqueueDownloadTask creates a download task for a file if one doesn't exist
func (s *Syncer) enqueueDownloadTask(file *domain.File) {
	// Re-read file to get latest cached status (avoid race with cacher)
	latestFile, err := s.files.GetByID(file.ID)
	if err != nil {
		s.logger.Warn("failed to re-read file for task enqueue",
			zap.String("path", file.Path),
			zap.Error(err))
		return
	}
	if latestFile == nil {
		s.logger.Warn("file not found for task enqueue",
			zap.String("path", file.Path))
		return
	}

	// Skip if file is already cached
	if latestFile.Cached {
		s.logger.Debug("file already cached, skipping task enqueue",
			zap.String("path", file.Path))
		return
	}

	// Check if task already exists
	hasTask, err := s.tasks.HasActiveTask(file.ID)
	if err != nil {
		s.logger.Warn("failed to check existing task",
			zap.String("path", file.Path),
			zap.Error(err))
		return
	}

	if hasTask {
		s.logger.Debug("download task already exists",
			zap.String("path", file.Path))
		return
	}

	task := &domain.DownloadTask{
		FileID:     file.ID,
		SynoPath:   file.Path,
		Priority:   file.Priority,
		Size:       file.Size,
		Status:     domain.TaskStatusPending,
		MaxRetries: 3,
	}

	if err := s.tasks.CreateTask(task); err != nil {
		if err == domain.ErrAlreadyExists {
			s.logger.Debug("download task already exists (race)",
				zap.String("path", file.Path))
			return
		}
		s.logger.Warn("failed to create download task",
			zap.String("path", file.Path),
			zap.Error(err))
		return
	}

	s.logger.Debug("download task enqueued",
		zap.String("path", file.Path),
		zap.Int("priority", file.Priority))
}

// createOrUpdateShareRecord creates or updates a share record
func (s *Syncer) createOrUpdateShareRecord(fileID int64, synoFileID int64, token string) {
	// Check if share already exists
	existingShare, err := s.shares.GetShareByToken(token)
	if err != nil {
		s.logger.Warn("failed to check existing share",
			zap.String("token", token),
			zap.Error(err))
		return
	}

	if existingShare != nil {
		// Update with advance sharing info
		s.updateShareWithAdvanceSharing(existingShare, synoFileID)
		return
	}

	// Get advanced sharing info
	var sharingLink, fullURL, password string
	var expiresAt *time.Time

	advInfo, err := s.drive.GetAdvanceSharing(synoFileID, "")
	if err != nil {
		s.logger.Warn("failed to get advance sharing info",
			zap.String("token", token),
			zap.Error(err))
	} else {
		sharingLink = advInfo.SharingLink
		fullURL = advInfo.URL
		password = advInfo.ProtectPassword
		expiresAt = advInfo.GetExpiresAt()
	}

	// Create new share record
	newShare := &domain.Share{
		SynoShareID: fmt.Sprintf("%d", synoFileID),
		Token:       token,
		SharingLink: sharingLink,
		URL:         fullURL,
		FileID:      fileID,
		Password:    password,
		ExpiresAt:   expiresAt,
		Revoked:     false,
	}

	if err := s.shares.CreateShare(newShare); err != nil {
		s.logger.Warn("failed to create share",
			zap.String("token", token),
			zap.Error(err))
		return
	}

	s.logger.Debug("share record created",
		zap.String("token", token),
		zap.String("sharing_link", sharingLink))
}

// updateShareWithAdvanceSharing updates a share with AdvanceSharing info
func (s *Syncer) updateShareWithAdvanceSharing(share *domain.Share, synoFileID int64) {
	advInfo, err := s.drive.GetAdvanceSharing(synoFileID, "")
	if err != nil {
		s.logger.Warn("failed to get advance sharing info for update",
			zap.String("token", share.Token),
			zap.Error(err))
		return
	}

	share.SharingLink = advInfo.SharingLink
	share.URL = advInfo.URL
	share.Password = advInfo.ProtectPassword
	share.ExpiresAt = advInfo.GetExpiresAt()

	if err := s.shares.UpdateShare(share); err != nil {
		s.logger.Warn("failed to update share",
			zap.String("token", share.Token),
			zap.Error(err))
		return
	}

	s.logger.Debug("share record updated",
		zap.String("token", share.Token),
		zap.Bool("has_password", advInfo.ProtectPassword != ""))
}
