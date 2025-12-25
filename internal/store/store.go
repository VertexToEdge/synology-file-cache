package store

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
	"github.com/vertextoedge/synology-file-cache/internal/logger"
)

// Store represents the SQLite database store
type Store struct {
	db *sql.DB
}

// Open opens a connection to the SQLite database
func Open(dbPath string) (*Store, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		// We'll let the OS create it when opening the file
	}

	// Open database
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
	if err := store.Migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	logger.Log.Info("Database opened successfully", "path", dbPath)
	return store, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// DB returns the underlying database connection
func (s *Store) DB() *sql.DB {
	return s.db
}

// Migrate creates or updates the database schema
func (s *Store) Migrate() error {
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

		// Create indexes for better query performance
		`CREATE INDEX IF NOT EXISTS idx_files_syno_file_id ON files(syno_file_id)`,
		`CREATE INDEX IF NOT EXISTS idx_files_path ON files(path)`,
		`CREATE INDEX IF NOT EXISTS idx_files_priority ON files(priority)`,
		`CREATE INDEX IF NOT EXISTS idx_files_cached ON files(cached)`,
		`CREATE INDEX IF NOT EXISTS idx_files_last_access ON files(last_access_in_cache_at)`,
		`CREATE INDEX IF NOT EXISTS idx_shares_token ON shares(token)`,
		`CREATE INDEX IF NOT EXISTS idx_shares_file_id ON shares(file_id)`,
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

	logger.Log.Info("Database migrations completed successfully")
	return nil
}