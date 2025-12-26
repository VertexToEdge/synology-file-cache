package sqlite

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/vertextoedge/synology-file-cache/internal/domain"
	"github.com/vertextoedge/synology-file-cache/internal/port"
)

// Store implements port.Store interface using SQLite
type Store struct {
	db *sql.DB
}

// Ensure Store implements port.Store
var _ port.Store = (*Store)(nil)

// Open opens a connection to the SQLite database
func Open(dbPath string) (*Store, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		// Directory will be created when the DB file is created
	}

	// Open database with WAL mode and busy timeout
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set pragmas for better performance
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -64000", // 64MB cache
		"PRAGMA temp_store = MEMORY",
		"PRAGMA busy_timeout = 5000",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set pragma %s: %w", pragma, err)
		}
	}

	store := &Store{db: db}

	// Run migrations
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return store, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Ping checks database connectivity
func (s *Store) Ping() error {
	return s.db.Ping()
}

// DB returns the underlying database connection (for backward compatibility)
func (s *Store) DB() *sql.DB {
	return s.db
}

// migrate creates or updates the database schema
func (s *Store) migrate() error {
	migrations := []string{
		// Create files table
		`CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			syno_file_id TEXT UNIQUE NOT NULL,
			path TEXT NOT NULL,
			size INTEGER NOT NULL DEFAULT 0,
			modified_at TIMESTAMP,
			accessed_at TIMESTAMP,
			starred BOOLEAN DEFAULT FALSE,
			shared BOOLEAN DEFAULT FALSE,
			last_sync_at TIMESTAMP,
			cached BOOLEAN DEFAULT FALSE,
			cache_path TEXT,
			priority INTEGER DEFAULT 5,
			last_access_in_cache_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Create shares table
		`CREATE TABLE IF NOT EXISTS shares (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			syno_share_id TEXT UNIQUE NOT NULL,
			token TEXT UNIQUE NOT NULL,
			file_id INTEGER NOT NULL,
			password TEXT,
			expires_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			revoked BOOLEAN DEFAULT FALSE,
			FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
		)`,

		// Create meta table for storing sync state
		`CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Create download_tasks table for task queue based downloads
		`CREATE TABLE IF NOT EXISTS download_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL,
			syno_path TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 5,
			size INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			worker_id TEXT,
			temp_file_path TEXT,
			bytes_downloaded INTEGER NOT NULL DEFAULT 0,
			retry_count INTEGER NOT NULL DEFAULT 0,
			max_retries INTEGER NOT NULL DEFAULT 3,
			next_retry_at TIMESTAMP,
			last_error TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			claimed_at TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
		)`,

		// Create indexes for better query performance
		`CREATE INDEX IF NOT EXISTS idx_files_syno_file_id ON files(syno_file_id)`,
		`CREATE INDEX IF NOT EXISTS idx_files_path ON files(path)`,
		`CREATE INDEX IF NOT EXISTS idx_files_priority ON files(priority)`,
		`CREATE INDEX IF NOT EXISTS idx_files_cached ON files(cached)`,
		`CREATE INDEX IF NOT EXISTS idx_files_last_access ON files(last_access_in_cache_at)`,
		`CREATE INDEX IF NOT EXISTS idx_shares_token ON shares(token)`,
		`CREATE INDEX IF NOT EXISTS idx_shares_file_id ON shares(file_id)`,
		`CREATE INDEX IF NOT EXISTS idx_download_tasks_status ON download_tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_download_tasks_priority ON download_tasks(priority, size)`,
		`CREATE INDEX IF NOT EXISTS idx_download_tasks_file_id ON download_tasks(file_id)`,
	}

	// Run migrations
	for _, migration := range migrations {
		if _, err := s.db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, migration)
		}
	}

	// Add new columns to shares table (safe ALTER TABLE - ignores if column exists)
	alterMigrations := []string{
		`ALTER TABLE shares ADD COLUMN sharing_link TEXT DEFAULT ''`,
		`ALTER TABLE shares ADD COLUMN url TEXT DEFAULT ''`,
	}

	for _, migration := range alterMigrations {
		// Ignore errors for ALTER TABLE as column may already exist
		s.db.Exec(migration)
	}

	// Migrate existing download_temp_files to download_tasks (one-time migration)
	s.migrateDownloadTempFiles()

	return nil
}

// migrateDownloadTempFiles migrates data from old download_temp_files to new download_tasks
func (s *Store) migrateDownloadTempFiles() {
	// Check if old table exists
	var tableName string
	err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='download_temp_files'").Scan(&tableName)
	if err != nil {
		// Table doesn't exist, nothing to migrate
		return
	}

	// Migrate existing records to download_tasks
	migrateQuery := `
		INSERT OR IGNORE INTO download_tasks (file_id, syno_path, temp_file_path, bytes_downloaded, status, priority, size)
		SELECT f.id, dtf.syno_path, dtf.temp_file_path, dtf.size_downloaded, 'pending', f.priority, f.size
		FROM download_temp_files dtf
		INNER JOIN files f ON dtf.syno_path = f.path
		WHERE NOT EXISTS (
			SELECT 1 FROM download_tasks dt
			WHERE dt.file_id = f.id AND dt.status IN ('pending', 'in_progress')
		)
	`
	s.db.Exec(migrateQuery)

	// Drop the old table
	s.db.Exec("DROP TABLE IF EXISTS download_temp_files")
}

// GetCacheStats returns cache statistics
func (s *Store) GetCacheStats() (*domain.CacheStats, error) {
	stats := &domain.CacheStats{}

	// Total files
	err := s.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&stats.TotalFiles)
	if err != nil {
		return nil, err
	}

	// Cached files
	err = s.db.QueryRow("SELECT COUNT(*) FROM files WHERE cached = TRUE").Scan(&stats.CachedFiles)
	if err != nil {
		return nil, err
	}

	// Total cache size
	var totalSize sql.NullInt64
	err = s.db.QueryRow("SELECT SUM(size) FROM files WHERE cached = TRUE").Scan(&totalSize)
	if err != nil {
		return nil, err
	}
	stats.CachedSizeBytes = totalSize.Int64

	// Active shares
	err = s.db.QueryRow("SELECT COUNT(*) FROM shares WHERE revoked = FALSE").Scan(&stats.ActiveShares)
	if err != nil {
		return nil, err
	}

	return stats, nil
}
