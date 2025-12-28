package port

import (
	"github.com/vertextoedge/synology-file-cache/internal/domain/repository"
)

// FileRepository is an alias to domain repository interface
// Kept for backward compatibility with existing code
type FileRepository = repository.FileRepository

// ShareRepository is an alias to domain repository interface
// Kept for backward compatibility with existing code
type ShareRepository = repository.ShareRepository

// DownloadTaskRepository is an alias to domain repository interface
// Kept for backward compatibility with existing code
type DownloadTaskRepository = repository.DownloadTaskRepository

// StatsRepository is an alias to domain repository interface
// Kept for backward compatibility with existing code
type StatsRepository = repository.StatsRepository

// Store is an alias to domain repository interface
// Kept for backward compatibility with existing code
type Store = repository.Store
