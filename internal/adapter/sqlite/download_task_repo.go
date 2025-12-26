package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
)

// CreateTask creates a new download task
func (s *Store) CreateTask(task *domain.DownloadTask) error {
	query := `
		INSERT INTO download_tasks (
			file_id, syno_path, priority, size, status, max_retries
		) VALUES (?, ?, ?, ?, 'pending', ?)
	`

	result, err := s.db.Exec(query,
		task.FileID, task.SynoPath, task.Priority, task.Size, task.MaxRetries)
	if err != nil {
		if isUniqueConstraintError(err) {
			return domain.ErrAlreadyExists
		}
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	task.ID = id
	task.Status = domain.TaskStatusPending
	return nil
}

// ClaimNextTask atomically claims the next pending task for a worker
func (s *Store) ClaimNextTask(workerID string) (*domain.DownloadTask, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Select next task to claim
	selectQuery := `
		SELECT id, file_id, syno_path, priority, size, status,
			   temp_file_path, bytes_downloaded, retry_count, max_retries,
			   last_error, created_at, updated_at
		FROM download_tasks
		WHERE status = 'pending'
		  AND (next_retry_at IS NULL OR next_retry_at <= datetime('now'))
		ORDER BY priority ASC, size ASC
		LIMIT 1
	`

	task := &domain.DownloadTask{}
	var tempPath, lastError sql.NullString

	err = tx.QueryRow(selectQuery).Scan(
		&task.ID, &task.FileID, &task.SynoPath, &task.Priority, &task.Size,
		&task.Status, &tempPath, &task.BytesDownloaded,
		&task.RetryCount, &task.MaxRetries, &lastError,
		&task.CreatedAt, &task.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if tempPath.Valid {
		task.TempFilePath = tempPath.String
	}
	if lastError.Valid {
		task.LastError = lastError.String
	}

	// Claim the task
	updateQuery := `
		UPDATE download_tasks
		SET status = 'in_progress',
			worker_id = ?,
			claimed_at = datetime('now'),
			updated_at = datetime('now')
		WHERE id = ?
	`

	_, err = tx.Exec(updateQuery, workerID, task.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	task.Status = domain.TaskStatusInProgress
	task.WorkerID = workerID
	now := time.Now()
	task.ClaimedAt = &now

	return task, nil
}

// GetTask retrieves a task by ID
func (s *Store) GetTask(id int64) (*domain.DownloadTask, error) {
	query := `
		SELECT id, file_id, syno_path, priority, size, status, worker_id,
			   temp_file_path, bytes_downloaded, retry_count, max_retries,
			   next_retry_at, last_error, created_at, claimed_at, updated_at
		FROM download_tasks
		WHERE id = ?
	`

	return s.scanTask(s.db.QueryRow(query, id))
}

// GetTaskByFileID retrieves an active task for a file
func (s *Store) GetTaskByFileID(fileID int64) (*domain.DownloadTask, error) {
	query := `
		SELECT id, file_id, syno_path, priority, size, status, worker_id,
			   temp_file_path, bytes_downloaded, retry_count, max_retries,
			   next_retry_at, last_error, created_at, claimed_at, updated_at
		FROM download_tasks
		WHERE file_id = ? AND status IN ('pending', 'in_progress')
	`

	return s.scanTask(s.db.QueryRow(query, fileID))
}

// HasActiveTask checks if a file has an active task
func (s *Store) HasActiveTask(fileID int64) (bool, error) {
	query := `
		SELECT COUNT(*) FROM download_tasks
		WHERE file_id = ? AND status IN ('pending', 'in_progress')
	`

	var count int
	err := s.db.QueryRow(query, fileID).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// UpdateTask updates a task's state
func (s *Store) UpdateTask(task *domain.DownloadTask) error {
	query := `
		UPDATE download_tasks
		SET status = ?, worker_id = ?, temp_file_path = ?, bytes_downloaded = ?,
			retry_count = ?, next_retry_at = ?, last_error = ?,
			claimed_at = ?, updated_at = datetime('now')
		WHERE id = ?
	`

	var workerID, tempPath, lastError sql.NullString
	var nextRetryAt, claimedAt sql.NullTime

	if task.WorkerID != "" {
		workerID = sql.NullString{String: task.WorkerID, Valid: true}
	}
	if task.TempFilePath != "" {
		tempPath = sql.NullString{String: task.TempFilePath, Valid: true}
	}
	if task.LastError != "" {
		lastError = sql.NullString{String: task.LastError, Valid: true}
	}
	if task.NextRetryAt != nil {
		nextRetryAt = sql.NullTime{Time: *task.NextRetryAt, Valid: true}
	}
	if task.ClaimedAt != nil {
		claimedAt = sql.NullTime{Time: *task.ClaimedAt, Valid: true}
	}

	_, err := s.db.Exec(query,
		task.Status, workerID, tempPath, task.BytesDownloaded,
		task.RetryCount, nextRetryAt, lastError, claimedAt, task.ID)

	return err
}

// UpdateProgress updates download progress
func (s *Store) UpdateProgress(taskID int64, bytesDownloaded int64, tempPath string) error {
	query := `
		UPDATE download_tasks
		SET bytes_downloaded = ?, temp_file_path = ?, updated_at = datetime('now')
		WHERE id = ?
	`

	_, err := s.db.Exec(query, bytesDownloaded, tempPath, taskID)
	return err
}

// CompleteTask removes a completed task
func (s *Store) CompleteTask(taskID int64) error {
	_, err := s.db.Exec("DELETE FROM download_tasks WHERE id = ?", taskID)
	return err
}

// FailTask marks a task as failed and schedules retry if possible
func (s *Store) FailTask(taskID int64, errMsg string, canRetry bool) error {
	if canRetry {
		// Get current retry count to calculate backoff
		var retryCount int
		err := s.db.QueryRow("SELECT retry_count FROM download_tasks WHERE id = ?", taskID).Scan(&retryCount)
		if err != nil {
			if err == sql.ErrNoRows {
				// Task was already deleted, nothing to do
				return nil
			}
			return err
		}

		// Calculate next retry time with exponential backoff
		backoffs := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute}
		backoffIdx := retryCount
		if backoffIdx >= len(backoffs) {
			backoffIdx = len(backoffs) - 1
		}
		nextRetry := time.Now().Add(backoffs[backoffIdx])

		query := `
			UPDATE download_tasks
			SET status = 'pending', worker_id = NULL, claimed_at = NULL,
				retry_count = retry_count + 1, next_retry_at = ?, last_error = ?,
				updated_at = datetime('now')
			WHERE id = ?
		`
		_, err = s.db.Exec(query, nextRetry, errMsg, taskID)
		return err
	}

	// Max retries exceeded, mark as failed
	query := `
		UPDATE download_tasks
		SET status = 'failed', worker_id = NULL, claimed_at = NULL,
			retry_count = retry_count + 1, last_error = ?,
			updated_at = datetime('now')
		WHERE id = ?
	`
	_, err := s.db.Exec(query, errMsg, taskID)
	return err
}

// ReleaseStaleInProgressTasks resets tasks stuck in in_progress state
func (s *Store) ReleaseStaleInProgressTasks(staleDuration time.Duration) (int, error) {
	cutoff := time.Now().Add(-staleDuration)

	query := `
		UPDATE download_tasks
		SET status = 'pending', worker_id = NULL, claimed_at = NULL,
			updated_at = datetime('now')
		WHERE status = 'in_progress' AND claimed_at < ?
	`

	result, err := s.db.Exec(query, cutoff)
	if err != nil {
		return 0, err
	}

	count, err := result.RowsAffected()
	return int(count), err
}

// GetQueueStats returns queue statistics
func (s *Store) GetQueueStats() (*domain.QueueStats, error) {
	stats := &domain.QueueStats{}

	// Get counts by status
	query := `
		SELECT status, COUNT(*), COALESCE(SUM(size), 0)
		FROM download_tasks
		GROUP BY status
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		var totalSize int64

		if err := rows.Scan(&status, &count, &totalSize); err != nil {
			return nil, err
		}

		switch status {
		case domain.TaskStatusPending:
			stats.PendingCount = count
			stats.TotalBytesQueued += totalSize
		case domain.TaskStatusInProgress:
			stats.InProgressCount = count
			stats.TotalBytesQueued += totalSize
		case domain.TaskStatusFailed:
			stats.FailedCount = count
		}
	}

	return stats, rows.Err()
}

// CleanupOldFailedTasks removes failed tasks older than the specified duration
func (s *Store) CleanupOldFailedTasks(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)

	result, err := s.db.Exec(
		"DELETE FROM download_tasks WHERE status = 'failed' AND updated_at < ?",
		cutoff)
	if err != nil {
		return 0, err
	}

	count, err := result.RowsAffected()
	return int(count), err
}

// GetOversizedTasks returns tasks whose file size exceeds maxSize
func (s *Store) GetOversizedTasks(maxSize int64) ([]*domain.DownloadTask, error) {
	query := `
		SELECT id, file_id, syno_path, priority, size, status, worker_id,
			   temp_file_path, bytes_downloaded, retry_count, max_retries,
			   next_retry_at, last_error, created_at, claimed_at, updated_at
		FROM download_tasks
		WHERE size > ? AND status IN ('pending', 'in_progress')
	`

	rows, err := s.db.Query(query, maxSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTasks(rows)
}

// DeleteTask removes a task by ID
func (s *Store) DeleteTask(taskID int64) error {
	_, err := s.db.Exec("DELETE FROM download_tasks WHERE id = ?", taskID)
	return err
}

// scanTask scans a single task row
func (s *Store) scanTask(row *sql.Row) (*domain.DownloadTask, error) {
	task := &domain.DownloadTask{}
	var workerID, tempPath, lastError sql.NullString
	var nextRetryAt, claimedAt sql.NullTime

	err := row.Scan(
		&task.ID, &task.FileID, &task.SynoPath, &task.Priority, &task.Size,
		&task.Status, &workerID, &tempPath, &task.BytesDownloaded,
		&task.RetryCount, &task.MaxRetries, &nextRetryAt, &lastError,
		&task.CreatedAt, &claimedAt, &task.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if workerID.Valid {
		task.WorkerID = workerID.String
	}
	if tempPath.Valid {
		task.TempFilePath = tempPath.String
	}
	if lastError.Valid {
		task.LastError = lastError.String
	}
	if nextRetryAt.Valid {
		task.NextRetryAt = &nextRetryAt.Time
	}
	if claimedAt.Valid {
		task.ClaimedAt = &claimedAt.Time
	}

	return task, nil
}

// scanTasks scans multiple task rows
func (s *Store) scanTasks(rows *sql.Rows) ([]*domain.DownloadTask, error) {
	var tasks []*domain.DownloadTask

	for rows.Next() {
		task := &domain.DownloadTask{}
		var workerID, tempPath, lastError sql.NullString
		var nextRetryAt, claimedAt sql.NullTime

		err := rows.Scan(
			&task.ID, &task.FileID, &task.SynoPath, &task.Priority, &task.Size,
			&task.Status, &workerID, &tempPath, &task.BytesDownloaded,
			&task.RetryCount, &task.MaxRetries, &nextRetryAt, &lastError,
			&task.CreatedAt, &claimedAt, &task.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if workerID.Valid {
			task.WorkerID = workerID.String
		}
		if tempPath.Valid {
			task.TempFilePath = tempPath.String
		}
		if lastError.Valid {
			task.LastError = lastError.String
		}
		if nextRetryAt.Valid {
			task.NextRetryAt = &nextRetryAt.Time
		}
		if claimedAt.Valid {
			task.ClaimedAt = &claimedAt.Time
		}

		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

// isUniqueConstraintError checks if the error is a unique constraint violation
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "UNIQUE constraint failed") ||
		strings.Contains(errStr, "duplicate key")
}
