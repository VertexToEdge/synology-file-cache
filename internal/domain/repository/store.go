package repository

// Store combines all repository interfaces
type Store interface {
	FileRepository
	ShareRepository
	DownloadTaskRepository
	StatsRepository

	// Close closes the database connection
	Close() error

	// Ping checks database connectivity
	Ping() error
}
