package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

	// File download endpoint (by share token)
	mux.HandleFunc("/f/", s.handleFileDownload)

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

// handleFileDownload handles file download requests by share token
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract token from path
	token := r.URL.Path[3:] // Remove "/f/" prefix
	if token == "" {
		http.Error(w, "Token required", http.StatusBadRequest)
		return
	}

	// TODO: Implement file download logic
	// For now, just return a placeholder response
	logger.Log.Debugw("File download requested", "token", token)

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "File download endpoint (token: %s) - Not implemented yet\n", token)
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