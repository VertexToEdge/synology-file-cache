# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Synology File Cache is a Go-based caching service that prefetches files from Synology NAS using the Drive HTTP API. It maintains a local cache of high-priority files (shared, starred, recently modified/accessed) to serve files when the NAS is offline.

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

# Run tests (when implemented)
go test ./...
go test -v ./internal/store/...  # Test specific package

# Format code
go fmt ./...

# Vet code for issues
go vet ./...
```

## Architecture

### Core Flow
1. **Syncer** periodically fetches file metadata from Synology NAS via HTTP API
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
- `internal/httpapi/`: HTTP server for file downloads (`/f/{token}`) and debug endpoints

### Planned Packages (Not Yet Implemented)
- `internal/fs/`: Local filesystem management and disk usage tracking
- `internal/synoapi/`: Synology Drive API client for authentication and file operations
- `internal/syncer/`: Synchronization engine for full and incremental syncs
- `internal/cacher/`: Prefetch and eviction logic with priority+LRU algorithm

### Database Schema

**files table**: Tracks all files with cache status
- `syno_file_id`: Synology's unique file identifier
- `priority`: 1-5 (lower = higher priority)
- `cached`: Whether file is locally cached
- `cache_path`: Local filesystem path when cached
- `last_access_in_cache_at`: For LRU eviction

**shares table**: Maps share tokens to files
- `token`: Synology-compatible share token
- `file_id`: References files.id
- `revoked`: Soft delete for expired shares

## Configuration

The application uses `config.yaml` with these key sections:
- `synology`: NAS connection details (URL, credentials)
- `cache`: Storage limits (GB and disk percentage), recent file windows
- `sync`: Timing intervals for full scan, incremental, and prefetch
- `http`: Server bind address
- `logging`: Level (debug/info/warn/error) and format (json/text)

## Key Implementation Details

### Priority System
Files are assigned priorities that determine cache order and eviction:
1. Shared files get priority 1
2. Starred files get priority 2
3. Recently modified get priority 3
4. Recently accessed get priority 4
5. Default priority is 5

When disk space is needed, files are evicted by highest priority number first, then by LRU within each priority level.

### Cache Directory Structure
Cache files mirror the Synology path structure under the configured `cache.root_dir`. The local path is stored in `files.cache_path`.

### HTTP API Behavior
- `/f/{token}`: Returns cached file if available, otherwise returns error (no passthrough to NAS)
- `/health`: Checks database connectivity
- `/debug/stats`: Returns cache statistics (total files, cached files, disk usage)
- `/debug/files`: Lists cached files with metadata

## Current Implementation Status

✅ **Implemented**:
- Configuration management with validation
- Structured logging with zap
- SQLite store with migrations and models
- HTTP server skeleton with basic endpoints
- Graceful shutdown handling

❌ **Not Yet Implemented**:
- Synology API client (authentication, file listing, downloads)
- Synchronization logic (full scan, incremental sync)
- File caching engine (prefetch, eviction)
- Local filesystem management
- Actual file serving from cache
- Share token validation against Synology

## Development Notes

- The application is designed to work behind a reverse proxy (Traefik/Caddy) that routes to NAS when online and this cache when offline
- Import paths use `github.com/vertextoedge/synology-file-cache`
- SQLite uses WAL mode for better concurrency
- All times are stored as UTC in the database