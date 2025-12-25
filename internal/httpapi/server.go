package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/logger"
	"github.com/vertextoedge/synology-file-cache/internal/store"
)

// Server represents the HTTP API server
type Server struct {
	store  *store.Store
	server *http.Server
}

// NewServer creates a new HTTP API server
func NewServer(bindAddr string, store *store.Store) *Server {
	s := &Server{
		store: store,
	}

	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// File download endpoints (by share token)
	mux.HandleFunc("/f/", s.handleFileDownload)       // Cache server format: /f/{token}
	mux.HandleFunc("/d/s/", s.handleSynologyDownload) // Synology Drive format: /d/s/{token}/{extra}

	// Debug endpoints
	mux.HandleFunc("/debug/files", s.handleDebugFiles)
	mux.HandleFunc("/debug/stats", s.handleDebugStats)

	s.server = &http.Server{
		Addr:         bindAddr,
		Handler:      s.withLogging(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// Start starts the HTTP server
func (s *Server) Start() error {
	logger.Log.Infof("Starting HTTP server on %s", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}
	return nil
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	logger.Log.Info("Stopping HTTP server")
	return s.server.Shutdown(ctx)
}

// withLogging adds request logging middleware
func (s *Server) withLogging(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		handler.ServeHTTP(rw, r)

		duration := time.Since(start)
		logger.Log.Debugw("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"status", rw.statusCode,
			"duration_ms", duration.Milliseconds(),
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check database connection
	if err := s.store.DB().Ping(); err != nil {
		logger.Log.Errorw("Health check failed", "error", err)
		http.Error(w, "Database connection failed", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// handleFileDownload handles file download requests by share token: /f/{token}
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract token from path
	token := strings.TrimPrefix(r.URL.Path, "/f/")
	if token == "" {
		http.Error(w, "Token required", http.StatusBadRequest)
		return
	}

	logger.Log.Debugw("File download requested", "token", token)
	s.serveFileByToken(w, r, token)
}

// handleSynologyDownload handles Synology Drive format URLs: /d/s/{token}/{extra}
func (s *Server) handleSynologyDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract token from path: /d/s/{token}/{extra}
	// The extra segment is ignored (it's used by Synology for additional routing)
	path := strings.TrimPrefix(r.URL.Path, "/d/s/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Token required", http.StatusBadRequest)
		return
	}

	token := parts[0]
	logger.Log.Debugw("Synology format download requested", "token", token, "path", r.URL.Path)

	// Reuse the same file serving logic
	s.serveFileByToken(w, r, token)
}

// serveFileByToken serves a cached file by its share token
func (s *Server) serveFileByToken(w http.ResponseWriter, r *http.Request, token string) {
	// Look up the share token and associated file
	file, share, err := s.store.GetFileByShareToken(token)
	if err != nil {
		logger.Log.Errorw("Failed to get file by share token", "token", token, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if file == nil || share == nil {
		http.Error(w, "Share not found", http.StatusNotFound)
		return
	}

	// Check if share is revoked
	if share.Revoked {
		http.Error(w, "Share has been revoked", http.StatusGone)
		return
	}

	// Check if share has expired
	if share.ExpiresAt != nil && share.ExpiresAt.Before(time.Now()) {
		http.Error(w, "Share has expired", http.StatusGone)
		return
	}

	// Check if file is cached
	if !file.Cached || !file.CachePath.Valid {
		http.Error(w, "File not cached", http.StatusServiceUnavailable)
		return
	}

	// Open the cached file
	cachePath := file.CachePath.String
	f, err := os.Open(cachePath)
	if err != nil {
		logger.Log.Errorw("Failed to open cached file", "path", cachePath, "error", err)
		http.Error(w, "File not available", http.StatusServiceUnavailable)
		return
	}
	defer f.Close()

	// Get file info for size
	stat, err := f.Stat()
	if err != nil {
		logger.Log.Errorw("Failed to stat cached file", "path", cachePath, "error", err)
		http.Error(w, "File not available", http.StatusServiceUnavailable)
		return
	}

	// Determine content type from file extension
	filename := filepath.Base(file.Path)
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Set headers
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))

	// Update last access time in database
	now := time.Now()
	file.LastAccessInCacheAt = &now
	if err := s.store.UpdateFile(file); err != nil {
		logger.Log.Warnw("Failed to update file access time", "error", err)
	}

	// Stream the file
	if _, err := io.Copy(w, f); err != nil {
		logger.Log.Errorw("Failed to stream file", "path", cachePath, "error", err)
		return
	}

	logger.Log.Infow("File served from cache",
		"token", token,
		"path", file.Path,
		"size", stat.Size())
}

// handleDebugFiles handles debug file listing requests
func (s *Server) handleDebugFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get cache stats
	stats, err := s.store.GetCacheStats()
	if err != nil {
		logger.Log.Errorw("Failed to get cache stats", "error", err)
		http.Error(w, "Failed to get cache stats", http.StatusInternalServerError)
		return
	}

	// Get some cached files for display
	files, err := s.store.GetFilesToCache(20)
	if err != nil {
		logger.Log.Errorw("Failed to get files", "error", err)
		http.Error(w, "Failed to get files", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"stats": stats,
		"files": files,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDebugStats handles debug statistics requests
func (s *Server) handleDebugStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := s.store.GetCacheStats()
	if err != nil {
		logger.Log.Errorw("Failed to get cache stats", "error", err)
		http.Error(w, "Failed to get cache stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}