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
go test -v ./internal/store/...  # Test specific package

# Format code
go fmt ./...

# Vet code for issues
go vet ./...
```

## Architecture

### Core Flow
1. **DriveSyncer** periodically fetches file metadata from Synology NAS via Drive API
2. **Store** (SQLite) maintains file metadata, share tokens, and cache state
3. **Cacher** downloads files based on priority and manages disk usage with LRU eviction
4. **HTTP API** serves cached files using Synology-compatible share tokens

### Package Structure

- `cmd/synology-file-cache/`: Entry point, initializes all components and manages lifecycle
- `internal/config/`: YAML configuration loading and validation using Viper
- `internal/store/`: SQLite database layer with File and Share models
  - Files tracked with priority (1=shared, 2=starred, 3=recent modified, 4=recent accessed)
  - Shares maintain Synology token compatibility
- `internal/logger/`: Structured logging with zap (JSON or text format)
- `internal/httpapi/`: HTTP server for file downloads and debug endpoints
- `internal/synoapi/`: Synology API client (Drive API + FileStation API)
  - Authentication with session management
  - Drive API: shared files, starred files, labels, recent files, file downloads
  - FileStation API: file info, favorites, share links
- `internal/syncer/`: DriveSyncer for full and incremental synchronization
  - Full sync: shared + starred + labeled + recent files
  - Incremental sync: same as full sync but runs more frequently
  - mtime-based cache invalidation
  - Configurable label exclusion
- `internal/cacher/`: Prefetch and eviction logic
  - Priority-based caching (lower number = higher priority)
  - LRU eviction within same priority level
  - Dual space limits: max_size_gb and max_disk_usage_percent
  - Rate-limited eviction (30-second minimum interval)
- `internal/fs/`: Local filesystem management
  - Atomic file writes (temp file + rename)
  - Disk usage tracking via syscall
  - Cache size calculation
- `internal/scanner/`: Recursive folder scanning for starred/labeled directories

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
- `revoked`: Soft delete for expired shares
- `expires_at`: Optional expiration date

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

sync:
  full_scan_interval: "1h"           # Full sync interval
  incremental_interval: "1m"         # Incremental sync interval
  prefetch_interval: "30s"           # Cacher prefetch interval
  exclude_labels: []                 # Labels to skip (e.g., ["temp", "no-cache"])

http:
  bind_addr: "0.0.0.0:8080"

logging:
  level: "info"   # debug, info, warn, error
  format: "json"  # json or text
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
2. If newer, set `cached = false` and clear `cache_path`
3. File becomes eligible for re-download in next cacher loop

### Space Management
Two-level enforcement before caching each file:
1. **Cache size check**: `current_cache + file_size <= max_size_gb`
2. **Disk usage check**: `disk_used_percent < max_disk_usage_percent`

If either limit exceeded, trigger eviction (rate-limited to 30-second intervals).

### HTTP API Endpoints
- `GET /f/{token}`: Serve cached file by permanent_link token
- `GET /d/s/{token}`: Serve cached file (alternative Synology format)
- `GET /d/s/{token}/{filename}`: Serve with filename in path
- `GET /health`: Health check (database connectivity)
- `GET /debug/stats`: Cache statistics (JSON)
- `GET /debug/files`: List cached files with metadata (JSON)

### Sync Flow
```
DriveSyncer.Start()
├── FullSync() immediately on start
├── fullScanLoop (every full_scan_interval)
│   └── syncSharedFiles + syncStarredFiles + syncLabeledFiles + syncRecentFiles
└── incrementalLoop (every incremental_interval)
    └── syncSharedFiles + syncStarredFiles + syncLabeledFiles + syncRecentFiles
```

Each sync method:
1. Fetches files from Synology API with pagination
2. Creates or updates file records in database
3. Checks mtime and invalidates cache if file was modified
4. Creates share records for files with permanent_link

## Current Implementation Status

✅ **Implemented**:
- Configuration management with validation
- Structured logging with zap
- SQLite store with migrations and models
- HTTP server with file serving endpoints
- Synology Drive API client (auth, files, shares, labels)
- DriveSyncer (full sync, incremental sync)
- mtime-based cache invalidation
- Label exclusion configuration
- Cacher with priority-based prefetch
- LRU eviction with rate limiting
- Local filesystem management with atomic writes
- Recursive folder scanning for starred directories
- Graceful shutdown handling

⚠️ **Partial/TODO**:
- FileStation API (basic implementation, not primary)
- Password-protected share handling
- Share expiration enforcement

## Development Notes

- The application is designed to work behind a reverse proxy (Traefik/Caddy) that routes to NAS when online and this cache when offline
- Import paths use `github.com/vertextoedge/synology-file-cache`
- SQLite uses WAL mode for better concurrency
- All times are stored as UTC in the database
- Config file contains secrets - use `config.yaml.example` as template, actual `config.yaml` is gitignored
