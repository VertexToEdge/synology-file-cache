package store

import (
	"database/sql"
	"time"
)

// File represents a file in the cache
type File struct {
	ID                   int64
	SynoFileID           string
	Path                 string
	Size                 int64
	ModifiedAt           *time.Time
	AccessedAt           *time.Time
	Starred              bool
	Shared               bool
	LastSyncAt           *time.Time
	Cached               bool
	CachePath            sql.NullString
	Priority             int
	LastAccessInCacheAt  *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Share represents a shared link for a file
type Share struct {
	ID           int64
	SynoShareID  string
	Token        string
	FileID       int64
	Password     sql.NullString
	ExpiresAt    *time.Time
	CreatedAt    time.Time
	Revoked      bool
}

// Priority levels for cache
const (
	PriorityShared         = 1
	PriorityStarred        = 2
	PriorityRecentModified = 3
	PriorityRecentAccessed = 4
	PriorityDefault        = 5
)

// CreateFile creates a new file record
func (s *Store) CreateFile(file *File) error {
	query := `
		INSERT INTO files (
			syno_file_id, path, size, modified_at, accessed_at,
			starred, shared, last_sync_at, cached, cache_path,
			priority, last_access_in_cache_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := s.db.Exec(
		query,
		file.SynoFileID, file.Path, file.Size, file.ModifiedAt, file.AccessedAt,
		file.Starred, file.Shared, file.LastSyncAt, file.Cached, file.CachePath,
		file.Priority, file.LastAccessInCacheAt,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	file.ID = id
	return nil
}

// GetFileByID retrieves a file by its ID
func (s *Store) GetFileByID(id int64) (*File, error) {
	query := `
		SELECT id, syno_file_id, path, size, modified_at, accessed_at,
			   starred, shared, last_sync_at, cached, cache_path,
			   priority, last_access_in_cache_at, created_at, updated_at
		FROM files
		WHERE id = ?
	`

	file := &File{}
	err := s.db.QueryRow(query, id).Scan(
		&file.ID, &file.SynoFileID, &file.Path, &file.Size, &file.ModifiedAt, &file.AccessedAt,
		&file.Starred, &file.Shared, &file.LastSyncAt, &file.Cached, &file.CachePath,
		&file.Priority, &file.LastAccessInCacheAt, &file.CreatedAt, &file.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return file, nil
}

// GetFileBySynoID retrieves a file by its Synology ID
func (s *Store) GetFileBySynoID(synoID string) (*File, error) {
	query := `
		SELECT id, syno_file_id, path, size, modified_at, accessed_at,
			   starred, shared, last_sync_at, cached, cache_path,
			   priority, last_access_in_cache_at, created_at, updated_at
		FROM files
		WHERE syno_file_id = ?
	`

	file := &File{}
	err := s.db.QueryRow(query, synoID).Scan(
		&file.ID, &file.SynoFileID, &file.Path, &file.Size, &file.ModifiedAt, &file.AccessedAt,
		&file.Starred, &file.Shared, &file.LastSyncAt, &file.Cached, &file.CachePath,
		&file.Priority, &file.LastAccessInCacheAt, &file.CreatedAt, &file.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return file, nil
}

// UpdateFile updates an existing file record
func (s *Store) UpdateFile(file *File) error {
	query := `
		UPDATE files SET
			path = ?, size = ?, modified_at = ?, accessed_at = ?,
			starred = ?, shared = ?, last_sync_at = ?, cached = ?,
			cache_path = ?, priority = ?, last_access_in_cache_at = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := s.db.Exec(
		query,
		file.Path, file.Size, file.ModifiedAt, file.AccessedAt,
		file.Starred, file.Shared, file.LastSyncAt, file.Cached,
		file.CachePath, file.Priority, file.LastAccessInCacheAt,
		file.ID,
	)

	return err
}

// DeleteFile deletes a file record
func (s *Store) DeleteFile(id int64) error {
	_, err := s.db.Exec("DELETE FROM files WHERE id = ?", id)
	return err
}

// GetFilesToCache returns files that need to be cached, ordered by priority
func (s *Store) GetFilesToCache(limit int) ([]*File, error) {
	query := `
		SELECT id, syno_file_id, path, size, modified_at, accessed_at,
			   starred, shared, last_sync_at, cached, cache_path,
			   priority, last_access_in_cache_at, created_at, updated_at
		FROM files
		WHERE cached = FALSE
		ORDER BY priority ASC, size ASC
		LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*File
	for rows.Next() {
		file := &File{}
		err := rows.Scan(
			&file.ID, &file.SynoFileID, &file.Path, &file.Size, &file.ModifiedAt, &file.AccessedAt,
			&file.Starred, &file.Shared, &file.LastSyncAt, &file.Cached, &file.CachePath,
			&file.Priority, &file.LastAccessInCacheAt, &file.CreatedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, nil
}

// GetEvictionCandidates returns cached files that can be evicted, ordered by priority and LRU
func (s *Store) GetEvictionCandidates(limit int) ([]*File, error) {
	query := `
		SELECT id, syno_file_id, path, size, modified_at, accessed_at,
			   starred, shared, last_sync_at, cached, cache_path,
			   priority, last_access_in_cache_at, created_at, updated_at
		FROM files
		WHERE cached = TRUE
		ORDER BY priority DESC, last_access_in_cache_at ASC
		LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*File
	for rows.Next() {
		file := &File{}
		err := rows.Scan(
			&file.ID, &file.SynoFileID, &file.Path, &file.Size, &file.ModifiedAt, &file.AccessedAt,
			&file.Starred, &file.Shared, &file.LastSyncAt, &file.Cached, &file.CachePath,
			&file.Priority, &file.LastAccessInCacheAt, &file.CreatedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, nil
}

// CreateShare creates a new share record
func (s *Store) CreateShare(share *Share) error {
	query := `
		INSERT INTO shares (syno_share_id, token, file_id, password, expires_at, revoked)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	result, err := s.db.Exec(
		query,
		share.SynoShareID, share.Token, share.FileID,
		share.Password, share.ExpiresAt, share.Revoked,
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

// GetShareByToken retrieves a share by its token
func (s *Store) GetShareByToken(token string) (*Share, error) {
	query := `
		SELECT id, syno_share_id, token, file_id, password, expires_at, created_at, revoked
		FROM shares
		WHERE token = ?
	`

	share := &Share{}
	err := s.db.QueryRow(query, token).Scan(
		&share.ID, &share.SynoShareID, &share.Token, &share.FileID,
		&share.Password, &share.ExpiresAt, &share.CreatedAt, &share.Revoked,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return share, nil
}

// UpdateShare updates an existing share record
func (s *Store) UpdateShare(share *Share) error {
	query := `
		UPDATE shares SET
			password = ?, expires_at = ?, revoked = ?
		WHERE id = ?
	`

	_, err := s.db.Exec(query, share.Password, share.ExpiresAt, share.Revoked, share.ID)
	return err
}

// GetFileByShareToken retrieves a file associated with a share token
func (s *Store) GetFileByShareToken(token string) (*File, *Share, error) {
	query := `
		SELECT
			f.id, f.syno_file_id, f.path, f.size, f.modified_at, f.accessed_at,
			f.starred, f.shared, f.last_sync_at, f.cached, f.cache_path,
			f.priority, f.last_access_in_cache_at, f.created_at, f.updated_at,
			s.id, s.syno_share_id, s.token, s.file_id, s.password, s.expires_at, s.created_at, s.revoked
		FROM shares s
		JOIN files f ON s.file_id = f.id
		WHERE s.token = ?
	`

	file := &File{}
	share := &Share{}

	err := s.db.QueryRow(query, token).Scan(
		&file.ID, &file.SynoFileID, &file.Path, &file.Size, &file.ModifiedAt, &file.AccessedAt,
		&file.Starred, &file.Shared, &file.LastSyncAt, &file.Cached, &file.CachePath,
		&file.Priority, &file.LastAccessInCacheAt, &file.CreatedAt, &file.UpdatedAt,
		&share.ID, &share.SynoShareID, &share.Token, &share.FileID,
		&share.Password, &share.ExpiresAt, &share.CreatedAt, &share.Revoked,
	)

	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	return file, share, nil
}

// GetCacheStats returns cache statistics
func (s *Store) GetCacheStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total files
	var totalFiles int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&totalFiles)
	if err != nil {
		return nil, err
	}
	stats["total_files"] = totalFiles

	// Cached files
	var cachedFiles int64
	err = s.db.QueryRow("SELECT COUNT(*) FROM files WHERE cached = TRUE").Scan(&cachedFiles)
	if err != nil {
		return nil, err
	}
	stats["cached_files"] = cachedFiles

	// Total cache size
	var totalSize sql.NullInt64
	err = s.db.QueryRow("SELECT SUM(size) FROM files WHERE cached = TRUE").Scan(&totalSize)
	if err != nil {
		return nil, err
	}
	stats["cached_size_bytes"] = totalSize.Int64

	// Active shares
	var activeShares int64
	err = s.db.QueryRow("SELECT COUNT(*) FROM shares WHERE revoked = FALSE").Scan(&activeShares)
	if err != nil {
		return nil, err
	}
	stats["active_shares"] = activeShares

	return stats, nil
}