package server

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// AdminHandler handles admin browser requests
type AdminHandler struct {
	store         port.Store
	logger        *zap.Logger
	cacheRootDir  string
	adminUsername string
	adminPassword string
}

// NewAdminHandler creates a new AdminHandler
func NewAdminHandler(store port.Store, adminUsername, adminPassword, cacheRootDir string, logger *zap.Logger) *AdminHandler {
	return &AdminHandler{
		store:         store,
		logger:        logger,
		cacheRootDir:  cacheRootDir,
		adminUsername: adminUsername,
		adminPassword: adminPassword,
	}
}

// HandleBrowse handles file browser requests
func (h *AdminHandler) HandleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract path from URL
	requestPath := strings.TrimPrefix(r.URL.Path, "/admin/browse")
	requestPath = strings.TrimPrefix(requestPath, "/")

	h.logger.Debug("admin browse request", zap.String("path", requestPath))

	// Build full filesystem path
	fullPath := filepath.Join(h.cacheRootDir, requestPath)

	// Security check: prevent directory traversal
	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(h.cacheRootDir)) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Check if path exists
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Path not found", http.StatusNotFound)
		} else {
			h.logger.Error("failed to stat path", zap.String("path", fullPath), zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// If it's a file, serve it directly
	if !info.IsDir() {
		h.serveFile(w, r, fullPath)
		return
	}

	// Read directory contents
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		h.logger.Error("failed to read directory", zap.String("path", fullPath), zap.Error(err))
		http.Error(w, "Failed to read directory", http.StatusInternalServerError)
		return
	}

	// Build file entry list
	fileEntries := h.buildFileEntries(entries, requestPath)

	// Sort: directories first, then alphabetically
	h.sortFileEntries(fileEntries)

	// Render HTML
	h.renderDirectoryListing(w, requestPath, fileEntries)
}

// HandleLogout handles logout by returning 401 to clear browser credentials
func (h *AdminHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
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

// fileEntry represents a file or directory entry
type fileEntry struct {
	Name                string
	Size                int64
	ModTime             time.Time
	IsDir               bool
	AccessedAt          *time.Time
	CreatedAt           *time.Time
	LastAccessInCacheAt *time.Time
}

// buildFileEntries creates file entries from directory entries
func (h *AdminHandler) buildFileEntries(entries []os.DirEntry, requestPath string) []fileEntry {
	var fileEntries []fileEntry

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fe := fileEntry{
			Name:    entry.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   entry.IsDir(),
		}

		// If it's a file, try to get metadata from DB
		if !entry.IsDir() {
			filePath := filepath.Join(requestPath, entry.Name())
			if dbFile, err := h.store.GetByPath(filePath); err == nil && dbFile != nil {
				fe.AccessedAt = dbFile.AccessedAt
				fe.CreatedAt = &dbFile.CreatedAt
				fe.LastAccessInCacheAt = dbFile.LastAccessInCacheAt
			}
		}

		fileEntries = append(fileEntries, fe)
	}

	return fileEntries
}

// sortFileEntries sorts entries: directories first, then alphabetically
func (h *AdminHandler) sortFileEntries(entries []fileEntry) {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			iIsDir := entries[i].IsDir
			jIsDir := entries[j].IsDir
			iName := strings.ToLower(entries[i].Name)
			jName := strings.ToLower(entries[j].Name)
			if (!iIsDir && jIsDir) || (iIsDir == jIsDir && iName > jName) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

// renderDirectoryListing renders the directory listing HTML
func (h *AdminHandler) renderDirectoryListing(w http.ResponseWriter, requestPath string, entries []fileEntry) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Build breadcrumb navigation
	breadcrumb := h.buildBreadcrumb(requestPath)

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

	for _, entry := range entries {
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

// buildBreadcrumb builds the breadcrumb navigation HTML
func (h *AdminHandler) buildBreadcrumb(requestPath string) string {
	if requestPath == "" {
		return `<a href="/admin/browse">📁 /</a>`
	}

	breadcrumb := `<a href="/admin/browse">📁</a> / `
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
		breadcrumb += `<a href="/admin/browse/` + currentPath + `">` + part + `</a> / `
	}

	// Remove trailing " / "
	if len(breadcrumb) > 3 {
		breadcrumb = breadcrumb[:len(breadcrumb)-3]
	}

	return breadcrumb
}

// serveFile serves a file from the filesystem
func (h *AdminHandler) serveFile(w http.ResponseWriter, r *http.Request, fullPath string) {
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			h.logger.Error("failed to open file", zap.String("path", fullPath), zap.Error(err))
			http.Error(w, "File not available", http.StatusInternalServerError)
		}
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		h.logger.Error("failed to stat file", zap.String("path", fullPath), zap.Error(err))
		http.Error(w, "File not available", http.StatusInternalServerError)
		return
	}

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
		h.logger.Error("failed to stream file", zap.String("path", fullPath), zap.Error(err))
		return
	}

	h.logger.Info("file served via admin access",
		zap.String("path", fullPath),
		zap.Int64("size", stat.Size()))
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
