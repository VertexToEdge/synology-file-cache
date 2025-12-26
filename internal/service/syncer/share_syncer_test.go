package syncer

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// mockDriveClient implements port.DriveClient for testing
type mockDriveClient struct {
	advanceSharingResp *port.AdvanceSharingInfo
	advanceSharingErr  error
}

func (m *mockDriveClient) GetSharedFiles(offset, limit int) (*port.DriveListResponse, error) {
	return nil, nil
}
func (m *mockDriveClient) GetStarredFiles(offset, limit int) (*port.DriveListResponse, error) {
	return nil, nil
}
func (m *mockDriveClient) GetRecentFiles(offset, limit int) (*port.DriveListResponse, error) {
	return nil, nil
}
func (m *mockDriveClient) GetLabels() ([]port.DriveLabel, error) { return nil, nil }
func (m *mockDriveClient) GetLabeledFiles(labelID string, offset, limit int) (*port.DriveListResponse, error) {
	return nil, nil
}
func (m *mockDriveClient) ListFiles(opts *port.DriveListOptions) (*port.DriveListResponse, error) {
	return nil, nil
}
func (m *mockDriveClient) DownloadFile(fileID int64, path string) (io.ReadCloser, string, int64, error) {
	return nil, "", 0, nil
}
func (m *mockDriveClient) DownloadFileWithRange(fileID int64, path string, rangeStart int64) (io.ReadCloser, string, int64, error) {
	return nil, "", 0, nil
}
func (m *mockDriveClient) GetAdvanceSharing(fileID int64, path string) (*port.AdvanceSharingInfo, error) {
	return m.advanceSharingResp, m.advanceSharingErr
}

// mockShareRepository implements port.ShareRepository for testing
type mockShareRepository struct {
	shares        map[string]*domain.Share
	getByTokenErr error
	createErr     error
	updateErr     error
}

func newMockShareRepository() *mockShareRepository {
	return &mockShareRepository{
		shares: make(map[string]*domain.Share),
	}
}

func (m *mockShareRepository) GetShareByToken(token string) (*domain.Share, error) {
	if m.getByTokenErr != nil {
		return nil, m.getByTokenErr
	}
	return m.shares[token], nil
}

func (m *mockShareRepository) GetFileByShareToken(token string) (*domain.File, *domain.Share, error) {
	share := m.shares[token]
	if share == nil {
		return nil, nil, nil
	}
	// Return a minimal file for testing
	return &domain.File{ID: share.FileID}, share, nil
}

func (m *mockShareRepository) CreateShare(share *domain.Share) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.shares[share.Token] = share
	return nil
}

func (m *mockShareRepository) UpdateShare(share *domain.Share) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.shares[share.Token] = share
	return nil
}

func TestShareSyncer_CreateOrUpdateShare_NewShare(t *testing.T) {
	logger := zap.NewNop()
	shareRepo := newMockShareRepository()
	expiresAt := time.Now().Add(24 * time.Hour)

	driveClient := &mockDriveClient{
		advanceSharingResp: &port.AdvanceSharingInfo{
			SharingLink:     "test-sharing-link",
			URL:             "https://example.com/share/abc",
			ProtectPassword: "secret123",
			DueDate:         expiresAt.Unix(),
		},
	}

	ss := NewShareSyncer(driveClient, shareRepo, logger)

	err := ss.CreateOrUpdateShare(100, 12345, "test-token")
	if err != nil {
		t.Fatalf("CreateOrUpdateShare() error = %v", err)
	}

	// Verify share was created
	share, exists := shareRepo.shares["test-token"]
	if !exists {
		t.Fatal("share was not created")
	}

	if share.FileID != 100 {
		t.Errorf("FileID = %v, want 100", share.FileID)
	}
	if share.SynoShareID != "12345" {
		t.Errorf("SynoShareID = %v, want '12345'", share.SynoShareID)
	}
	if share.Token != "test-token" {
		t.Errorf("Token = %v, want 'test-token'", share.Token)
	}
	if share.SharingLink != "test-sharing-link" {
		t.Errorf("SharingLink = %v, want 'test-sharing-link'", share.SharingLink)
	}
	if share.Password != "secret123" {
		t.Errorf("Password = %v, want 'secret123'", share.Password)
	}
	if share.URL != "https://example.com/share/abc" {
		t.Errorf("URL = %v, want 'https://example.com/share/abc'", share.URL)
	}
}

func TestShareSyncer_CreateOrUpdateShare_ExistingShare(t *testing.T) {
	logger := zap.NewNop()
	shareRepo := newMockShareRepository()

	// Pre-populate an existing share
	existingShare := &domain.Share{
		ID:          1,
		Token:       "existing-token",
		FileID:      100,
		SynoShareID: "12345",
		SharingLink: "old-link",
		Password:    "",
	}
	shareRepo.shares["existing-token"] = existingShare

	driveClient := &mockDriveClient{
		advanceSharingResp: &port.AdvanceSharingInfo{
			SharingLink:     "updated-link",
			URL:             "https://example.com/share/updated",
			ProtectPassword: "new-password",
		},
	}

	ss := NewShareSyncer(driveClient, shareRepo, logger)

	err := ss.CreateOrUpdateShare(100, 12345, "existing-token")
	if err != nil {
		t.Fatalf("CreateOrUpdateShare() error = %v", err)
	}

	// Verify share was updated
	share := shareRepo.shares["existing-token"]
	if share.SharingLink != "updated-link" {
		t.Errorf("SharingLink = %v, want 'updated-link'", share.SharingLink)
	}
	if share.Password != "new-password" {
		t.Errorf("Password = %v, want 'new-password'", share.Password)
	}
}

func TestShareSyncer_CreateOrUpdateShare_DriveAPIError(t *testing.T) {
	logger := zap.NewNop()
	shareRepo := newMockShareRepository()

	driveClient := &mockDriveClient{
		advanceSharingErr: errors.New("API error"),
	}

	ss := NewShareSyncer(driveClient, shareRepo, logger)

	// Should still create share even if AdvanceSharing fails
	err := ss.CreateOrUpdateShare(100, 12345, "test-token")
	if err != nil {
		t.Fatalf("CreateOrUpdateShare() error = %v", err)
	}

	// Share should be created with minimal info
	share := shareRepo.shares["test-token"]
	if share == nil {
		t.Fatal("share was not created")
	}
	if share.SharingLink != "" {
		t.Errorf("SharingLink should be empty on API error, got %v", share.SharingLink)
	}
}

func TestShareSyncer_CreateOrUpdateShare_GetShareError(t *testing.T) {
	logger := zap.NewNop()
	shareRepo := newMockShareRepository()
	shareRepo.getByTokenErr = errors.New("database error")

	driveClient := &mockDriveClient{}

	ss := NewShareSyncer(driveClient, shareRepo, logger)

	err := ss.CreateOrUpdateShare(100, 12345, "test-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestShareSyncer_CreateOrUpdateShare_CreateError(t *testing.T) {
	logger := zap.NewNop()
	shareRepo := newMockShareRepository()
	shareRepo.createErr = errors.New("create error")

	driveClient := &mockDriveClient{
		advanceSharingResp: &port.AdvanceSharingInfo{
			SharingLink: "test-link",
		},
	}

	ss := NewShareSyncer(driveClient, shareRepo, logger)

	err := ss.CreateOrUpdateShare(100, 12345, "test-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestShareSyncer_UpdateWithAdvanceSharing(t *testing.T) {
	logger := zap.NewNop()
	shareRepo := newMockShareRepository()
	expiresAt := time.Now().Add(48 * time.Hour)

	driveClient := &mockDriveClient{
		advanceSharingResp: &port.AdvanceSharingInfo{
			SharingLink:     "new-sharing-link",
			URL:             "https://example.com/share/new",
			ProtectPassword: "updated-password",
			DueDate:         expiresAt.Unix(),
		},
	}

	share := &domain.Share{
		ID:          1,
		Token:       "test-token",
		FileID:      100,
		SharingLink: "old-link",
	}

	ss := NewShareSyncer(driveClient, shareRepo, logger)

	err := ss.UpdateWithAdvanceSharing(share, 12345)
	if err != nil {
		t.Fatalf("UpdateWithAdvanceSharing() error = %v", err)
	}

	if share.SharingLink != "new-sharing-link" {
		t.Errorf("SharingLink = %v, want 'new-sharing-link'", share.SharingLink)
	}
	if share.URL != "https://example.com/share/new" {
		t.Errorf("URL = %v, want 'https://example.com/share/new'", share.URL)
	}
	if share.Password != "updated-password" {
		t.Errorf("Password = %v, want 'updated-password'", share.Password)
	}
}

func TestShareSyncer_UpdateWithAdvanceSharing_APIError(t *testing.T) {
	logger := zap.NewNop()
	shareRepo := newMockShareRepository()

	driveClient := &mockDriveClient{
		advanceSharingErr: errors.New("API error"),
	}

	share := &domain.Share{
		ID:          1,
		Token:       "test-token",
		SharingLink: "original-link",
	}

	ss := NewShareSyncer(driveClient, shareRepo, logger)

	err := ss.UpdateWithAdvanceSharing(share, 12345)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Original share should not be modified
	if share.SharingLink != "original-link" {
		t.Errorf("SharingLink should remain unchanged, got %v", share.SharingLink)
	}
}

func TestShareSyncer_UpdateWithAdvanceSharing_UpdateError(t *testing.T) {
	logger := zap.NewNop()
	shareRepo := newMockShareRepository()
	shareRepo.updateErr = errors.New("update error")

	driveClient := &mockDriveClient{
		advanceSharingResp: &port.AdvanceSharingInfo{
			SharingLink: "new-link",
		},
	}

	share := &domain.Share{
		ID:    1,
		Token: "test-token",
	}

	ss := NewShareSyncer(driveClient, shareRepo, logger)

	err := ss.UpdateWithAdvanceSharing(share, 12345)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
