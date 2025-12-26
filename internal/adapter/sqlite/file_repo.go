package sqlite

import (
	"database/sql"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
)

// GetByID retrieves a file by its internal ID
func (s *Store) GetByID(id int64) (*domain.File, error) {
	query := `
		SELECT id, syno_file_id, path, size, modified_at, accessed_at,
			   starred, shared, last_sync_at, cached, cache_path,
			   priority, last_access_in_cache_at, created_at, updated_at
		FROM files
		WHERE id = ?
	`

	file := &domain.File{}
	var cachePath sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&file.ID, &file.SynoFileID, &file.Path, &file.Size, &file.ModifiedAt, &file.AccessedAt,
		&file.Starred, &file.Shared, &file.LastSyncAt, &file.Cached, &cachePath,
		&file.Priority, &file.LastAccessInCacheAt, &file.CreatedAt, &file.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if cachePath.Valid {
		file.CachePath = cachePath.String
	}

	return file, nil
}

// GetBySynoID retrieves a file by its Synology file ID
func (s *Store) GetBySynoID(synoID string) (*domain.File, error) {
	query := `
		SELECT id, syno_file_id, path, size, modified_at, accessed_at,
			   starred, shared, last_sync_at, cached, cache_path,
			   priority, last_access_in_cache_at, created_at, updated_at
		FROM files
		WHERE syno_file_id = ?
	`

	file := &domain.File{}
	var cachePath sql.NullString

	err := s.db.QueryRow(query, synoID).Scan(
		&file.ID, &file.SynoFileID, &file.Path, &file.Size, &file.ModifiedAt, &file.AccessedAt,
		&file.Starred, &file.Shared, &file.LastSyncAt, &file.Cached, &cachePath,
		&file.Priority, &file.LastAccessInCacheAt, &file.CreatedAt, &file.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if cachePath.Valid {
		file.CachePath = cachePath.String
	}

	return file, nil
}

// GetByPath retrieves a file by its path
func (s *Store) GetByPath(path string) (*domain.File, error) {
	query := `
		SELECT id, syno_file_id, path, size, modified_at, accessed_at,
			   starred, shared, last_sync_at, cached, cache_path,
			   priority, last_access_in_cache_at, created_at, updated_at
		FROM files
		WHERE path = ?
	`

	file := &domain.File{}
	var cachePath sql.NullString

	err := s.db.QueryRow(query, path).Scan(
		&file.ID, &file.SynoFileID, &file.Path, &file.Size, &file.ModifiedAt, &file.AccessedAt,
		&file.Starred, &file.Shared, &file.LastSyncAt, &file.Cached, &cachePath,
		&file.Priority, &file.LastAccessInCacheAt, &file.CreatedAt, &file.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if cachePath.Valid {
		file.CachePath = cachePath.String
	}

	return file, nil
}

// Create creates a new file record
func (s *Store) Create(file *domain.File) error {
	query := `
		INSERT INTO files (
			syno_file_id, path, size, modified_at, accessed_at,
			starred, shared, last_sync_at, cached, cache_path,
			priority, last_access_in_cache_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	var cachePath sql.NullString
	if file.CachePath != "" {
		cachePath = sql.NullString{String: file.CachePath, Valid: true}
	}

	result, err := s.db.Exec(
		query,
		file.SynoFileID, file.Path, file.Size, file.ModifiedAt, file.AccessedAt,
		file.Starred, file.Shared, file.LastSyncAt, file.Cached, cachePath,
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

// Update updates an existing file record (including cache status)
// Use UpdateMetadata for syncer to avoid overwriting cache status
func (s *Store) Update(file *domain.File) error {
	query := `
		UPDATE files SET
			path = ?, size = ?, modified_at = ?, accessed_at = ?,
			starred = ?, shared = ?, last_sync_at = ?, cached = ?,
			cache_path = ?, priority = ?, last_access_in_cache_at = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	var cachePath sql.NullString
	if file.CachePath != "" {
		cachePath = sql.NullString{String: file.CachePath, Valid: true}
	}

	_, err := s.db.Exec(
		query,
		file.Path, file.Size, file.ModifiedAt, file.AccessedAt,
		file.Starred, file.Shared, file.LastSyncAt, file.Cached,
		cachePath, file.Priority, file.LastAccessInCacheAt,
		file.ID,
	)

	return err
}

// UpdateMetadata updates file metadata without touching cache status fields
// This prevents race condition between syncer and cacher
func (s *Store) UpdateMetadata(file *domain.File) error {
	query := `
		UPDATE files SET
			path = ?, size = ?, modified_at = ?, accessed_at = ?,
			starred = ?, shared = ?, last_sync_at = ?, priority = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := s.db.Exec(
		query,
		file.Path, file.Size, file.ModifiedAt, file.AccessedAt,
		file.Starred, file.Shared, file.LastSyncAt, file.Priority,
		file.ID,
	)

	return err
}

// InvalidateCache sets cached=false for a file (used when source file is modified)
func (s *Store) InvalidateCache(fileID int64) error {
	query := `
		UPDATE files SET
			cached = FALSE, cache_path = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := s.db.Exec(query, fileID)
	return err
}

// Delete deletes a file record by ID
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec("DELETE FROM files WHERE id = ?", id)
	return err
}

// GetEvictionCandidates returns cached files that can be evicted
func (s *Store) GetEvictionCandidates(limit int) ([]*domain.File, error) {
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

	return s.scanFiles(rows)
}

// scanFiles is a helper to scan multiple file rows
func (s *Store) scanFiles(rows *sql.Rows) ([]*domain.File, error) {
	var files []*domain.File

	for rows.Next() {
		file := &domain.File{}
		var cachePath sql.NullString

		err := rows.Scan(
			&file.ID, &file.SynoFileID, &file.Path, &file.Size, &file.ModifiedAt, &file.AccessedAt,
			&file.Starred, &file.Shared, &file.LastSyncAt, &file.Cached, &cachePath,
			&file.Priority, &file.LastAccessInCacheAt, &file.CreatedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if cachePath.Valid {
			file.CachePath = cachePath.String
		}

		files = append(files, file)
	}

	return files, rows.Err()
}
