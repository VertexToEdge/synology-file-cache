package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// sessionEntry represents an authenticated session for a share
type sessionEntry struct {
	token     string
	expiresAt time.Time
}

// FileHandler handles file download requests
type FileHandler struct {
	store    port.Store
	logger   *zap.Logger
	sessions map[string]sessionEntry
	sessLock sync.RWMutex
}

// NewFileHandler creates a new FileHandler
func NewFileHandler(store port.Store, logger *zap.Logger) *FileHandler {
	return &FileHandler{
		store:    store,
		logger:   logger,
		sessions: make(map[string]sessionEntry),
	}
}

// HandleDownload handles file download by share token: /f/{token}
func (h *FileHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := strings.TrimPrefix(r.URL.Path, "/f/")
	if token == "" {
		http.Error(w, "Token required", http.StatusBadRequest)
		return
	}

	h.logger.Debug("file download requested", zap.String("token", token))
	h.serveFileByToken(w, r, token)
}

// HandleSynologyDownload handles Synology Drive format: /d/s/{token}/{extra}
func (h *FileHandler) HandleSynologyDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/d/s/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Token required", http.StatusBadRequest)
		return
	}

	token := parts[0]
	h.logger.Debug("synology download requested", zap.String("token", token))
	h.serveFileByToken(w, r, token)
}

// serveFileByToken serves a cached file by its share token
func (h *FileHandler) serveFileByToken(w http.ResponseWriter, r *http.Request, token string) {
	file, share, err := h.store.GetFileByShareToken(token)
	if err != nil {
		h.logger.Error("failed to get file by share token", zap.String("token", token), zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if file == nil || share == nil {
		http.Error(w, "Share not found", http.StatusNotFound)
		return
	}

	if share.Revoked {
		http.Error(w, "Share has been revoked", http.StatusGone)
		return
	}

	if share.ExpiresAt != nil && share.ExpiresAt.Before(time.Now()) {
		http.Error(w, "Share has expired", http.StatusGone)
		return
	}

	// Check password
	if share.HasPassword() {
		if !h.verifySharePassword(w, r, token, share.Password) {
			return
		}
	}

	if !file.Cached || file.CachePath == "" {
		http.Error(w, "File not cached", http.StatusServiceUnavailable)
		return
	}

	// Open cached file
	f, err := os.Open(file.CachePath)
	if err != nil {
		h.logger.Error("failed to open cached file", zap.String("path", file.CachePath), zap.Error(err))
		http.Error(w, "File not available", http.StatusServiceUnavailable)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		h.logger.Error("failed to stat cached file", zap.String("path", file.CachePath), zap.Error(err))
		http.Error(w, "File not available", http.StatusServiceUnavailable)
		return
	}

	// Determine content type
	filename := filepath.Base(file.Path)
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Set headers
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))

	// Update last access time
	now := time.Now()
	file.LastAccessInCacheAt = &now
	if err := h.store.Update(file); err != nil {
		h.logger.Warn("failed to update file access time", zap.Error(err))
	}

	// Stream file
	if _, err := io.Copy(w, f); err != nil {
		h.logger.Error("failed to stream file", zap.String("path", file.CachePath), zap.Error(err))
		return
	}

	h.logger.Info("file served from cache",
		zap.String("token", token),
		zap.String("path", file.Path),
		zap.Int64("size", stat.Size()))
}

// verifySharePassword verifies password for protected share
func (h *FileHandler) verifySharePassword(w http.ResponseWriter, r *http.Request, shareToken, correctPassword string) bool {
	// Check session cookie
	if cookie, err := r.Cookie("share_session"); err == nil {
		if h.validateSession(cookie.Value, shareToken) {
			return true
		}
	}

	// Check Basic Auth
	_, password, ok := r.BasicAuth()
	if ok {
		if subtle.ConstantTimeCompare([]byte(password), []byte(correctPassword)) == 1 {
			sessionID := h.createSession(shareToken)
			h.setSessionCookie(w, sessionID)
			return true
		}
		http.Error(w, "Invalid password", http.StatusForbidden)
		return false
	}

	w.Header().Set("WWW-Authenticate", `Basic realm="Password Protected Share"`)
	http.Error(w, "Password required", http.StatusUnauthorized)
	return false
}

// createSession creates a new session
func (h *FileHandler) createSession(shareToken string) string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		hash := sha256.Sum256([]byte(shareToken + time.Now().String()))
		bytes = hash[:]
	}
	sessionID := hex.EncodeToString(bytes)

	h.sessLock.Lock()
	defer h.sessLock.Unlock()

	if len(h.sessions) > 1000 {
		h.cleanExpiredSessions()
	}

	h.sessions[sessionID] = sessionEntry{
		token:     shareToken,
		expiresAt: time.Now().Add(24 * time.Hour),
	}

	return sessionID
}

// validateSession validates a session
func (h *FileHandler) validateSession(sessionID, shareToken string) bool {
	h.sessLock.RLock()
	defer h.sessLock.RUnlock()

	session, exists := h.sessions[sessionID]
	if !exists {
		return false
	}

	if time.Now().After(session.expiresAt) {
		return false
	}

	return session.token == shareToken
}

// setSessionCookie sets the session cookie
func (h *FileHandler) setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "share_session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   false,
	})
}

// cleanExpiredSessions removes expired sessions
func (h *FileHandler) cleanExpiredSessions() {
	now := time.Now()
	for id, session := range h.sessions {
		if now.After(session.expiresAt) {
			delete(h.sessions, id)
		}
	}
}
