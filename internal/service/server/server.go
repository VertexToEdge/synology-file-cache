package server

import (
	"context"
	"net/http"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// Config contains HTTP server configuration
type Config struct {
	BindAddr           string
	AdminUsername      string
	AdminPassword      string
	EnableAdminBrowser bool
	CacheRootDir       string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
}

// DefaultConfig returns default server configuration
func DefaultConfig() *Config {
	return &Config{
		BindAddr:     "0.0.0.0:8080",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// Server represents the HTTP API server
type Server struct {
	config       *Config
	store        port.Store
	logger       *zap.Logger
	server       *http.Server
	fileHandler  *FileHandler
	adminHandler *AdminHandler
	debugHandler *DebugHandler
}

// New creates a new HTTP server
func New(cfg *Config, store port.Store, logger *zap.Logger) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	s := &Server{
		config: cfg,
		store:  store,
		logger: logger,
	}

	s.fileHandler = NewFileHandler(store, logger)
	s.adminHandler = NewAdminHandler(store, cfg.AdminUsername, cfg.AdminPassword, cfg.CacheRootDir, logger)
	s.debugHandler = NewDebugHandler(store, logger)

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", s.handleHealth)

	// File download endpoints
	mux.HandleFunc("/f/", s.fileHandler.HandleDownload)
	mux.HandleFunc("/d/s/", s.fileHandler.HandleSynologyDownload)

	// Admin browser
	if cfg.EnableAdminBrowser {
		adminAuth := BasicAuthMiddleware(cfg.AdminUsername, cfg.AdminPassword, logger)
		mux.HandleFunc("/admin/browse", adminAuth(s.adminHandler.HandleBrowse))
		mux.HandleFunc("/admin/browse/", adminAuth(s.adminHandler.HandleBrowse))
		mux.HandleFunc("/admin/logout", s.adminHandler.HandleLogout)
	}

	// Debug endpoints
	mux.HandleFunc("/debug/files", s.debugHandler.HandleFiles)
	mux.HandleFunc("/debug/stats", s.debugHandler.HandleStats)

	s.server = &http.Server{
		Addr:         cfg.BindAddr,
		Handler:      LoggingMiddleware(logger)(mux),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return s
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.logger.Info("starting HTTP server", zap.String("addr", s.server.Addr))
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("stopping HTTP server")
	return s.server.Shutdown(ctx)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.store.Ping(); err != nil {
		s.logger.Error("health check failed", zap.Error(err))
		http.Error(w, "Database connection failed", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"healthy","time":"` + time.Now().Format(time.RFC3339) + `"}`))
}
