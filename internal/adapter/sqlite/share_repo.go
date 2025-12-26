package sqlite

import (
	"database/sql"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
)

// GetShareByToken retrieves a share by its token
func (s *Store) GetShareByToken(token string) (*domain.Share, error) {
	query := `
		SELECT id, syno_share_id, token, sharing_link, url, file_id, password, expires_at, created_at, revoked
		FROM shares
		WHERE token = ?
	`

	share := &domain.Share{}
	var password sql.NullString
	var sharingLink, url sql.NullString

	err := s.db.QueryRow(query, token).Scan(
		&share.ID, &share.SynoShareID, &share.Token, &sharingLink, &url,
		&share.FileID, &password, &share.ExpiresAt, &share.CreatedAt, &share.Revoked,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if password.Valid {
		share.Password = password.String
	}
	if sharingLink.Valid {
		share.SharingLink = sharingLink.String
	}
	if url.Valid {
		share.URL = url.String
	}

	return share, nil
}

// GetFileByShareToken retrieves both the file and share by share token
func (s *Store) GetFileByShareToken(token string) (*domain.File, *domain.Share, error) {
	query := `
		SELECT
			f.id, f.syno_file_id, f.path, f.size, f.modified_at, f.accessed_at,
			f.starred, f.shared, f.last_sync_at, f.cached, f.cache_path,
			f.priority, f.last_access_in_cache_at, f.created_at, f.updated_at,
			s.id, s.syno_share_id, s.token, s.sharing_link, s.url, s.file_id, s.password, s.expires_at, s.created_at, s.revoked
		FROM shares s
		JOIN files f ON s.file_id = f.id
		WHERE s.token = ?
	`

	file := &domain.File{}
	share := &domain.Share{}
	var cachePath sql.NullString
	var password sql.NullString
	var sharingLink, url sql.NullString

	err := s.db.QueryRow(query, token).Scan(
		&file.ID, &file.SynoFileID, &file.Path, &file.Size, &file.ModifiedAt, &file.AccessedAt,
		&file.Starred, &file.Shared, &file.LastSyncAt, &file.Cached, &cachePath,
		&file.Priority, &file.LastAccessInCacheAt, &file.CreatedAt, &file.UpdatedAt,
		&share.ID, &share.SynoShareID, &share.Token, &sharingLink, &url, &share.FileID,
		&password, &share.ExpiresAt, &share.CreatedAt, &share.Revoked,
	)

	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	if cachePath.Valid {
		file.CachePath = cachePath.String
	}
	if password.Valid {
		share.Password = password.String
	}
	if sharingLink.Valid {
		share.SharingLink = sharingLink.String
	}
	if url.Valid {
		share.URL = url.String
	}

	return file, share, nil
}

// CreateShare creates a new share record
func (s *Store) CreateShare(share *domain.Share) error {
	query := `
		INSERT INTO shares (syno_share_id, token, sharing_link, url, file_id, password, expires_at, revoked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	var password sql.NullString
	if share.Password != "" {
		password = sql.NullString{String: share.Password, Valid: true}
	}

	result, err := s.db.Exec(
		query,
		share.SynoShareID, share.Token, share.SharingLink, share.URL,
		share.FileID, password, share.ExpiresAt, share.Revoked,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	share.ID = id
	return nil
}

// UpdateShare updates an existing share record
func (s *Store) UpdateShare(share *domain.Share) error {
	query := `
		UPDATE shares SET
			sharing_link = ?, url = ?, password = ?, expires_at = ?, revoked = ?
		WHERE id = ?
	`

	var password sql.NullString
	if share.Password != "" {
		password = sql.NullString{String: share.Password, Valid: true}
	}

	_, err := s.db.Exec(query, share.SharingLink, share.URL, password, share.ExpiresAt, share.Revoked, share.ID)
	return err
}
