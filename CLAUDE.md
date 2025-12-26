# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Synology File Cache is a Go-based caching service that prefetches files from Synology NAS using the Drive HTTP API. It maintains a local cache of high-priority files (shared, starred, labeled, recently modified) to serve files when the NAS is offline.

## Build and Development Commands

```bash
# Build the application
go build -o synology-file-cache ./cmd/synology-file-cache

# Run the application
./synology-file-cache -config config.yaml

# Run with custom config
./synology-file-cache -config /path/to/config.yaml

# Download dependencies
go mod download
go mod tidy

# Run tests
go test ./...
go test -v ./internal/adapter/sqlite/...  # Test specific package

# Format code
go fmt ./...

# Vet code for issues
go vet ./...
```

## Architecture

This project follows **Hexagonal Architecture (Port-Adapter Pattern)** with clear separation of concerns:

### Core Flow
1. **Syncer** periodically fetches file metadata from Synology NAS via Drive API
2. **Store** (SQLite adapter) maintains file metadata, share tokens, and cache state
3. **Cacher** downloads files based on priority and manages disk usage with LRU eviction
4. **Server** serves cached files using Synology-compatible share tokens

### Package Structure

```
internal/
├── domain/                    # Domain models (pure business logic)
│   ├── file.go               # File, CacheStats entities
│   ├── download_task.go      # DownloadTask entity for task queue
│   ├── share.go              # Share entity
│   ├── priority.go           # Priority constants
│   └── errors.go             # Domain errors

├── port/                      # Interface definitions (ports)
│   ├── repository.go         # FileRepository, ShareRepository, DownloadTaskRepository, Store
│   ├── synology.go           # SynologyClient, DriveClient interfaces
│   └── filesystem.go         # FileSystem interface

├── adapter/                   # External system adapters
│   ├── sqlite/               # SQLite implementation
│   │   ├── store.go          # DB connection, migrations, GetCacheStats
│   │   ├── file_repo.go      # FileRepository implementation
│   │   ├── share_repo.go     # ShareRepository implementation
│   │   └── download_task_repo.go  # DownloadTaskRepository implementation
│   │
│   ├── synology/             # Synology API client
│   │   ├── client.go         # Common HTTP client + session management
│   │   ├── drive.go          # Drive API implementation
│   │   └── types.go          # API response types
│   │
│   └── filesystem/           # Filesystem implementation
│       ├── manager.go        # FileSystem interface implementation
│       ├── disk_unix.go      # Unix disk usage (syscall.Statfs)
│       └── disk_windows.go   # Windows disk usage (kernel32.dll)

├── service/                   # Application services
│   ├── syncer/               # Synchronization service
│   │   ├── syncer.go         # Main Syncer with config, Start/Stop
│   │   ├── file_sync.go      # Template method for file sync (eliminates duplication)
│   │   └── scanner.go        # Directory scanner (integrated)
│   │
│   ├── cacher/               # Caching service
│   │   ├── cacher.go         # Main Cacher with worker pool
│   │   ├── downloader.go     # Download worker with resume support
│   │   └── evictor.go        # Eviction policy with rate limiting
│   │
│   └── server/               # HTTP server
│       ├── server.go         # Server setup + routing
│       ├── file_handler.go   # File download handlers (/f/, /d/s/)
│       ├── admin_handler.go  # Admin browser (/admin/)
│       ├── debug_handler.go  # Debug endpoints (/debug/)
│       └── middleware.go     # Logging, BasicAuth middleware

├── config/                    # Configuration management
└── logger/                    # Structured logging with zap
```

### Database Schema

**files table**: Tracks all files with cache status
- `syno_file_id`: Synology's unique file identifier
- `path`: File path on NAS
- `size`: File size in bytes
- `priority`: 1-5 (lower = higher priority)
- `cached`: Whether file is locally cached
- `cache_path`: Local filesystem path when cached
- `last_access_in_cache_at`: For LRU eviction (updated on file serve)
- `modified_at`: File modification time (for cache invalidation)
- `starred`, `shared`: Boolean flags

**shares table**: Maps share tokens to files
- `token`: Synology-compatible share token (permanent_link)
- `sharing_link`: Full sharing link from AdvanceSharing API
- `file_id`: References files.id
- `password`: Password for protected shares
- `revoked`: Soft delete for expired shares
- `expires_at`: Optional expiration date

**download_tasks table**: Task queue for download management
- `file_id`: References files.id
- `syno_path`: Synology file path (denormalized for easy access)
- `priority`: Task priority (copy from file for ordering)
- `size`: File size for space planning
- `status`: pending, in_progress, failed
- `worker_id`: Worker identifier for debugging
- `temp_file_path`: Local temporary file path (e.g., `/cache/path/file.zip.downloading`)
- `bytes_downloaded`: Bytes downloaded so far (for resume)
- `retry_count`: Current retry attempt
- `max_retries`: Maximum retries (default: 3)
- `next_retry_at`: Scheduled retry time (exponential backoff: 1m, 5m, 30m)
- `last_error`: Last error message for diagnostics
- `claimed_at`: When worker claimed the task

## Configuration

The application uses `config.yaml` (see `config.yaml.example`):

```yaml
synology:
  base_url: "https://your-nas.example.com"
  username: "username"
  password: "password"
  skip_tls_verify: false

cache:
  root_dir: "./cache-data"
  max_size_gb: 50                    # Cache size limit
  max_disk_usage_percent: 50         # Disk usage limit
  recent_modified_days: 30           # Include files modified within N days
  concurrent_downloads: 3            # Parallel download workers
  eviction_interval: "30s"           # Eviction check interval
  buffer_size_mb: 4                  # Download buffer size
  stale_task_timeout: "30m"          # Timeout for in-progress tasks (worker recovery)
  progress_update_interval: "10s"    # How often to update download progress

sync:
  full_scan_interval: "1h"           # Full sync interval
  incremental_interval: "1m"         # Incremental sync interval
  exclude_labels: []                 # Labels to skip (e.g., ["temp", "no-cache"])

http:
  bind_addr: "0.0.0.0:8080"
  enable_admin_browser: false        # Admin file browser (uses synology credentials)
  read_timeout: "30s"                # HTTP read timeout
  write_timeout: "30s"               # HTTP write timeout
  idle_timeout: "60s"                # HTTP idle timeout

logging:
  level: "info"   # debug, info, warn, error
  format: "json"  # json or text

database:
  path: ""                           # DB path (defaults to cache.root_dir/cache.db)
  cache_size_mb: 64                  # SQLite cache size
  busy_timeout_ms: 5000              # SQLite busy timeout
```

## Key Implementation Details

### Priority System
Files are assigned priorities that determine cache order and eviction:
1. **Priority 1**: Shared files (shared with others)
2. **Priority 2**: Starred files + Labeled files
3. **Priority 3**: Recently modified files
4. **Priority 4**: Recently accessed files (reserved)
5. **Priority 5**: Default (not actively tracked)

Caching order: `ORDER BY priority ASC, size ASC` (high priority + small files first)
Eviction order: `ORDER BY priority DESC, last_access_in_cache_at ASC` (low priority + LRU first)

### Cache Invalidation
When syncer detects a file's mtime has changed:
1. Compare new mtime with stored `modified_at`
2. If newer, call `file.InvalidateCache()` which sets `cached = false` and clears `cache_path`
3. Syncer enqueues a new download task for the file
4. Workers pick up the task and re-download the updated file

### Space Management
Two-level enforcement before caching each file:
1. **Cache size check**: `current_cache + file_size <= max_size_gb`
2. **Disk usage check**: `disk_used_percent < max_disk_usage_percent`

If either limit exceeded, trigger eviction (rate-limited by `eviction_interval`).

### Template Method Pattern (Syncer)
The `syncFilesWithFetcher` template method eliminates ~200 lines of code duplication:
```go
type FileFetcher func(offset, limit int) (*port.DriveListResponse, error)

func (s *Syncer) syncFilesWithFetcher(ctx context.Context, fetcher FileFetcher, priority int) (int, error)
```

This allows `syncSharedFiles`, `syncStarredFiles`, `syncLabeledFiles`, `syncRecentFiles` to share common pagination and file processing logic.

### Task Queue Based Download System
The Cacher uses a push-based task queue for downloads:

**Flow:**
1. **Syncer enqueues tasks**: When processing files, Syncer creates download tasks for uncached files
2. **Workers claim tasks**: Worker pool atomically claims pending tasks (priority ASC, size ASC)
3. **Download with resume**: If task has `bytes_downloaded > 0`, resume using HTTP Range header
4. **Progress tracking**: Periodic progress updates to database for recovery
5. **Retry on failure**: Exponential backoff (1m, 5m, 30m) with max 3 retries
6. **Stale task recovery**: Tasks stuck in `in_progress` longer than `stale_task_timeout` are reset to `pending`

**Benefits:**
- Interrupted downloads automatically resume on server restart
- Centralized task state management
- Priority-based download ordering
- Automatic retry with backoff

### HTTP API Endpoints
- `GET /f/{token}`: Serve cached file by permanent_link token
- `GET /d/s/{token}`: Serve cached file (alternative Synology format)
- `GET /d/s/{token}/{filename}`: Serve with filename in path
- `GET /health`: Health check (database connectivity)
- `GET /debug/stats`: Cache statistics (JSON)
- `GET /debug/files`: List cached files with metadata (JSON)
- `GET /admin/browse`: Admin file browser (requires Basic Auth)

### Sync Flow
```
Syncer.Start()
├── FullSync() immediately on start
├── fullScanLoop (every full_scan_interval)
│   └── syncFilesWithFetcher (shared, starred, labeled, recent)
└── incrementalLoop (every incremental_interval)
    └── syncFilesWithFetcher (shared, starred, labeled, recent)
```

## Current Implementation Status

✅ **Implemented**:
- Hexagonal Architecture with port-adapter pattern
- Domain models with business logic (File, Share, DownloadTask, Priority)
- Interface-based dependency injection
- Configuration management with validation (all timeouts, buffer sizes configurable)
- Structured logging with zap
- SQLite adapter with migrations and repository implementations
- HTTP server with file serving, admin browser, debug endpoints
- Password-protected share handling with session management
- Synology Drive API client (auth, files, shares, labels)
- Syncer with template method pattern (eliminates code duplication)
- **Task queue based download system** with worker pool
- Automatic download resume on server restart
- Retry with exponential backoff (1m, 5m, 30m)
- Stale task recovery for crashed workers
- mtime-based cache invalidation with automatic re-download
- LRU eviction with rate limiting
- Local filesystem management with atomic writes
- Platform-specific disk usage (Windows/Unix)
- HTTP Range request-based resume download
- Graceful shutdown handling

⚠️ **TODO**:
- Metrics collection (Prometheus)
- Integration tests
- Share expiration enforcement enhancement

## Development Notes

- The application is designed to work behind a reverse proxy (Traefik/Caddy) that routes to NAS when online and this cache when offline
- Import paths use `github.com/vertextoedge/synology-file-cache`
- SQLite uses WAL mode for better concurrency
- All times are stored as UTC in the database
- Config file contains secrets - use `config.yaml.example` as template, actual `config.yaml` is gitignored
- Windows support is included (disk_windows.go uses kernel32.dll)
- Interfaces in `port/` package allow easy mocking for tests
