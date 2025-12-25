package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/logger"
	"github.com/vertextoedge/synology-file-cache/internal/store"
)

// sessionEntry represents an authenticated session for a share
type sessionEntry struct {
	token     string    // share token
	expiresAt time.Time // session expiration
}

// Server represents the HTTP API server
type Server struct {
	store              *store.Store
	server             *http.Server
	sessions           map[string]sessionEntry // session ID -> session entry
	sessLock           sync.RWMutex
	adminUsername      string
	adminPassword      string
	enableAdminBrowser bool
	cacheRootDir       string
}

// NewServer creates a new HTTP API server
func NewServer(bindAddr string, store *store.Store, adminUsername, adminPassword string, enableAdminBrowser bool, cacheRootDir string) *Server {
	s := &Server{
		store:              store,
		sessions:           make(map[string]sessionEntry),
		adminUsername:      adminUsername,
		adminPassword:      adminPassword,
		enableAdminBrowser: enableAdminBrowser,
		cacheRootDir:       cacheRootDir,
	}

	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// File download endpoints (by share token)
	mux.HandleFunc("/f/", s.handleFileDownload)       // Cache server format: /f/{token}
	mux.HandleFunc("/d/s/", s.handleSynologyDownload) // Synology Drive format: /d/s/{token}/{extra}

	// Admin file browser endpoints (requires basic auth)
	if enableAdminBrowser {
		mux.HandleFunc("/admin/browse", s.withAdminAuth(s.handleAdminBrowse))
		mux.HandleFunc("/admin/browse/", s.withAdminAuth(s.handleAdminBrowse))
		mux.HandleFunc("/admin/logout", s.handleAdminLogout)
	}

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

	// Check if share requires password
	if share.Password.Valid && share.Password.String != "" {
		if !s.verifySharePassword(w, r, token, share.Password.String) {
			return
		}
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

// verifySharePassword verifies the password for a password-protected share
// Returns true if verified, false if not (and sends appropriate HTTP response)
func (s *Server) verifySharePassword(w http.ResponseWriter, r *http.Request, shareToken, correctPassword string) bool {
	// 1. Check session cookie first
	if cookie, err := r.Cookie("share_session"); err == nil {
		if s.validateSession(cookie.Value, shareToken) {
			return true
		}
	}

	// 2. Check HTTP Basic Auth
	_, password, ok := r.BasicAuth()
	if ok {
		// Use constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(password), []byte(correctPassword)) == 1 {
			// Create session for future requests
			sessionID := s.createSession(shareToken)
			s.setSessionCookie(w, sessionID)
			return true
		}
		http.Error(w, "Invalid password", http.StatusForbidden)
		return false
	}

	// 3. No credentials provided - request authentication
	w.Header().Set("WWW-Authenticate", `Basic realm="Password Protected Share"`)
	http.Error(w, "Password required", http.StatusUnauthorized)
	return false
}

// createSession creates a new session for an authenticated share
func (s *Server) createSession(shareToken string) string {
	// Generate random session ID
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to hash-based ID if random fails
		hash := sha256.Sum256([]byte(shareToken + time.Now().String()))
		bytes = hash[:]
	}
	sessionID := hex.EncodeToString(bytes)

	s.sessLock.Lock()
	defer s.sessLock.Unlock()

	// Clean up expired sessions periodically
	if len(s.sessions) > 1000 {
		s.cleanExpiredSessions()
	}

	s.sessions[sessionID] = sessionEntry{
		token:     shareToken,
		expiresAt: time.Now().Add(24 * time.Hour), // Session valid for 24 hours
	}

	return sessionID
}

// validateSession checks if a session is valid for the given share token
func (s *Server) validateSession(sessionID, shareToken string) bool {
	s.sessLock.RLock()
	defer s.sessLock.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return false
	}

	if time.Now().After(session.expiresAt) {
		return false
	}

	return session.token == shareToken
}

// setSessionCookie sets the session cookie in the response
func (s *Server) setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "share_session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   86400, // 24 hours
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   false, // Set to true if using HTTPS
	})
}

// cleanExpiredSessions removes expired sessions from the map
func (s *Server) cleanExpiredSessions() {
	now := time.Now()
	for id, session := range s.sessions {
		if now.After(session.expiresAt) {
			delete(s.sessions, id)
		}
	}
}

// withAdminAuth wraps a handler with HTTP Basic Auth middleware
func (s *Server) withAdminAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Admin Access"`)
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Use constant-time comparison to prevent timing attacks
		validUsername := subtle.ConstantTimeCompare([]byte(username), []byte(s.adminUsername)) == 1
		validPassword := subtle.ConstantTimeCompare([]byte(password), []byte(s.adminPassword)) == 1

		if !validUsername || !validPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="Admin Access"`)
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			logger.Log.Warnw("Failed admin authentication attempt",
				"username", username,
				"remote_addr", r.RemoteAddr,
			)
			return
		}

		handler(w, r)
	}
}

// handleAdminBrowse handles file browser requests for admin
func (s *Server) handleAdminBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract path from URL
	requestPath := strings.TrimPrefix(r.URL.Path, "/admin/browse")
	requestPath = strings.TrimPrefix(requestPath, "/")

	logger.Log.Debugw("Admin browse request", "path", requestPath)

	// Build full filesystem path
	fullPath := filepath.Join(s.cacheRootDir, requestPath)

	// Security check: prevent directory traversal
	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(s.cacheRootDir)) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Check if path exists
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Path not found", http.StatusNotFound)
		} else {
			logger.Log.Errorw("Failed to stat path", "path", fullPath, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// If it's a file, serve it directly
	if !info.IsDir() {
		s.serveFile(w, r, fullPath)
		return
	}

	// Read directory contents
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		logger.Log.Errorw("Failed to read directory", "path", fullPath, "error", err)
		http.Error(w, "Failed to read directory", http.StatusInternalServerError)
		return
	}

	// Build file entry list
	type FileEntry struct {
		Name                string
		Size                int64
		ModTime             time.Time
		IsDir               bool
		AccessedAt          *time.Time
		CreatedAt           *time.Time
		LastAccessInCacheAt *time.Time
	}

	var fileEntries []FileEntry
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileEntry := FileEntry{
			Name:    entry.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   entry.IsDir(),
		}

		// If it's a file, try to get metadata from DB
		if !entry.IsDir() {
			filePath := filepath.Join(requestPath, entry.Name())
			if dbFile, err := s.store.GetFileByPath(filePath); err == nil && dbFile != nil {
				fileEntry.AccessedAt = dbFile.AccessedAt
				fileEntry.CreatedAt = &dbFile.CreatedAt
				fileEntry.LastAccessInCacheAt = dbFile.LastAccessInCacheAt
			}
		}

		fileEntries = append(fileEntries, fileEntry)
	}

	// Sort: directories first, then alphabetically
	for i := 0; i < len(fileEntries); i++ {
		for j := i + 1; j < len(fileEntries); j++ {
			iIsDir := fileEntries[i].IsDir
			jIsDir := fileEntries[j].IsDir
			iName := strings.ToLower(fileEntries[i].Name)
			jName := strings.ToLower(fileEntries[j].Name)
			if (!iIsDir && jIsDir) || (iIsDir == jIsDir && iName > jName) {
				fileEntries[i], fileEntries[j] = fileEntries[j], fileEntries[i]
			}
		}
	}

	// Render HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Build breadcrumb navigation
	var breadcrumb string
	if requestPath == "" {
		breadcrumb = `<a href="/admin/browse">📁 /</a>`
	} else {
		breadcrumb = `<a href="/admin/browse">📁</a> / `
		// Always use "/" for URL paths, regardless of OS
		parts := strings.Split(strings.ReplaceAll(requestPath, string(filepath.Separator), "/"), "/")
		currentPath := ""
		for _, part := range parts {
			if part == "" {
				continue
			}
			if currentPath != "" {
				currentPath += "/"
			}
			currentPath += part

			// All parts are clickable
			breadcrumb += `<a href="/admin/browse/` + currentPath + `">` + part + `</a> / `
		}
		// Remove trailing " / "
		if len(breadcrumb) > 3 {
			breadcrumb = breadcrumb[:len(breadcrumb)-3]
		}
	}

	displayPath := "/" + requestPath
	if displayPath == "//" {
		displayPath = "/"
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>File Browser - ` + displayPath + `</title>
    <style>
        body { font-family: sans-serif; margin: 20px; }
        .header { display: flex; justify-content: space-between; align-items: center; border-bottom: 2px solid #333; padding-bottom: 10px; margin-bottom: 20px; }
        .breadcrumb { margin: 0; font-size: 24px; font-weight: normal; }
        .breadcrumb a { color: #0066cc; text-decoration: none; }
        .breadcrumb a:hover { text-decoration: underline; }
        .logout-btn {
            background-color: #dc3545;
            color: white;
            border: none;
            padding: 8px 16px;
            border-radius: 4px;
            cursor: pointer;
            text-decoration: none;
            font-size: 14px;
        }
        .logout-btn:hover { background-color: #c82333; }
        table { border-collapse: collapse; width: 100%; margin-top: 20px; }
        th, td { text-align: left; padding: 8px 12px; border-bottom: 1px solid #ddd; }
        th { background-color: #f0f0f0; font-weight: bold; }
        tr:hover { background-color: #f9f9f9; }
        a { color: #0066cc; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .size { text-align: right; }
        .parent { font-weight: bold; }
    </style>
</head>
<body>
    <div class="header">
        <h1 class="breadcrumb">` + breadcrumb + `</h1>
        <a href="/admin/logout" class="logout-btn">Logout</a>
    </div>
    <table>
`

	// Add parent directory link
	if requestPath != "" {
		parentPath := filepath.Dir(requestPath)
		if parentPath == "." {
			parentPath = ""
		}
		html += `        <tr class="parent">
            <td colspan="6"><a href="/admin/browse/` + parentPath + `">📁 ..</a></td>
        </tr>
`
	}

	html += `        <tr>
            <th>Name</th>
            <th>Size</th>
            <th>Modified</th>
            <th>Accessed</th>
            <th>Cached At</th>
            <th>Last Served</th>
        </tr>
`

	for _, entry := range fileEntries {
		icon := "📄"
		targetPath := filepath.Join(requestPath, entry.Name)
		link := "/admin/browse/" + targetPath

		sizeStr := "-"
		if !entry.IsDir {
			sizeStr = formatSize(entry.Size)
		} else {
			icon = "📁"
		}

		modTimeStr := entry.ModTime.Format("2006-01-02 15:04:05")

		accessedStr := "-"
		if entry.AccessedAt != nil {
			accessedStr = entry.AccessedAt.Format("2006-01-02 15:04:05")
		}

		createdStr := "-"
		if entry.CreatedAt != nil {
			createdStr = entry.CreatedAt.Format("2006-01-02 15:04:05")
		}

		lastServedStr := "-"
		if entry.LastAccessInCacheAt != nil {
			lastServedStr = entry.LastAccessInCacheAt.Format("2006-01-02 15:04:05")
		}

		html += `        <tr>
            <td><a href="` + link + `">` + icon + ` ` + entry.Name + `</a></td>
            <td class="size">` + sizeStr + `</td>
            <td>` + modTimeStr + `</td>
            <td>` + accessedStr + `</td>
            <td>` + createdStr + `</td>
            <td>` + lastServedStr + `</td>
        </tr>
`
	}

	html += `    </table>
</body>
</html>`

	w.Write([]byte(html))
}

// serveFile serves a file from the filesystem
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, fullPath string) {
	// Open file
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			logger.Log.Errorw("Failed to open file", "path", fullPath, "error", err)
			http.Error(w, "File not available", http.StatusInternalServerError)
		}
		return
	}
	defer f.Close()

	// Get file info
	stat, err := f.Stat()
	if err != nil {
		logger.Log.Errorw("Failed to stat file", "path", fullPath, "error", err)
		http.Error(w, "File not available", http.StatusInternalServerError)
		return
	}

	// Don't serve directories
	if stat.IsDir() {
		http.Error(w, "Cannot download directory", http.StatusBadRequest)
		return
	}

	// Determine content type
	filename := filepath.Base(fullPath)
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Set headers
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))

	// Stream file
	if _, err := io.Copy(w, f); err != nil {
		logger.Log.Errorw("Failed to stream file", "path", fullPath, "error", err)
		return
	}

	logger.Log.Infow("File served via admin access",
		"path", fullPath,
		"size", stat.Size())
}

// handleAdminLogout handles logout by returning 401 to clear browser credentials
func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Admin Access"`)
	w.WriteHeader(http.StatusUnauthorized)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Logged Out</title>
    <style>
        body { font-family: sans-serif; margin: 50px; text-align: center; }
        h1 { color: #333; }
        p { color: #666; margin-top: 20px; }
        a { color: #0066cc; text-decoration: none; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <h1>✓ Logged Out</h1>
    <p>You have been successfully logged out.</p>
    <p><a href="/admin/browse">Log in again</a></p>
</body>
</html>`
	w.Write([]byte(html))
}

// formatSize formats file size in human-readable format
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}