package syncer

import (
	"fmt"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// ShareSyncer handles share record creation and updates
type ShareSyncer struct {
	drive  port.DriveClient
	shares port.ShareRepository
	logger *zap.Logger
}

// NewShareSyncer creates a new ShareSyncer
func NewShareSyncer(drive port.DriveClient, shares port.ShareRepository, logger *zap.Logger) *ShareSyncer {
	return &ShareSyncer{
		drive:  drive,
		shares: shares,
		logger: logger,
	}
}

// CreateOrUpdateShare creates or updates a share record for a file
func (ss *ShareSyncer) CreateOrUpdateShare(fileID int64, synoFileID int64, token string) error {
	// Check if share already exists
	existingShare, err := ss.shares.GetShareByToken(token)
	if err != nil {
		ss.logger.Warn("failed to check existing share",
			zap.String("token", token),
			zap.Error(err))
		return fmt.Errorf("failed to check existing share: %w", err)
	}

	if existingShare != nil {
		// Update with advance sharing info
		return ss.UpdateWithAdvanceSharing(existingShare, synoFileID)
	}

	// Get advanced sharing info
	var sharingLink, fullURL, password string
	var expiresAt *time.Time

	advInfo, err := ss.drive.GetAdvanceSharing(synoFileID, "")
	if err != nil {
		ss.logger.Warn("failed to get advance sharing info",
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

	if err := ss.shares.CreateShare(newShare); err != nil {
		ss.logger.Warn("failed to create share",
			zap.String("token", token),
			zap.Error(err))
		return fmt.Errorf("failed to create share: %w", err)
	}

	ss.logger.Debug("share record created",
		zap.String("token", token),
		zap.String("sharing_link", sharingLink))

	return nil
}

// UpdateWithAdvanceSharing updates a share with AdvanceSharing info from the API
func (ss *ShareSyncer) UpdateWithAdvanceSharing(share *domain.Share, synoFileID int64) error {
	advInfo, err := ss.drive.GetAdvanceSharing(synoFileID, "")
	if err != nil {
		ss.logger.Warn("failed to get advance sharing info for update",
			zap.String("token", share.Token),
			zap.Error(err))
		return fmt.Errorf("failed to get advance sharing info: %w", err)
	}

	share.SharingLink = advInfo.SharingLink
	share.URL = advInfo.URL
	share.Password = advInfo.ProtectPassword
	share.ExpiresAt = advInfo.GetExpiresAt()

	if err := ss.shares.UpdateShare(share); err != nil {
		ss.logger.Warn("failed to update share",
			zap.String("token", share.Token),
			zap.Error(err))
		return fmt.Errorf("failed to update share: %w", err)
	}

	ss.logger.Debug("share record updated",
		zap.String("token", share.Token),
		zap.Bool("has_password", advInfo.ProtectPassword != ""))

	return nil
}
