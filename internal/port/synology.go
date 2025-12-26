package port

import (
	"encoding/json"
	"io"
	"time"
)

// SynologyClient defines the interface for Synology API authentication
type SynologyClient interface {
	// Login authenticates with the Synology NAS
	Login() error

	// Logout logs out from the Synology NAS
	Logout() error

	// IsLoggedIn returns true if the client has an active session
	IsLoggedIn() bool
}

// DriveFile represents a file from Synology Drive API
type DriveFile struct {
	ID            json.Number `json:"file_id"`
	Name          string      `json:"name"`
	Path          string      `json:"display_path"`
	ContentType   string      `json:"content_type"` // "dir" or "file"
	Size          int64       `json:"size"`
	MTime         int64       `json:"content_mtime"` // Content modification time (Unix timestamp)
	ATime         int64       `json:"access_time"`   // Access time
	Starred       bool        `json:"starred"`
	Shared        bool        `json:"adv_shared"`
	PermanentLink string      `json:"permanent_link"` // Share token for adv_shared files
	Labels        []DriveLabel `json:"labels,omitempty"`
}

// GetID returns the file ID as int64
func (f *DriveFile) GetID() int64 {
	id, _ := f.ID.Int64()
	return id
}

// GetIDString returns the file ID as string
func (f *DriveFile) GetIDString() string {
	return f.ID.String()
}

// IsDir returns true if the file is a directory
func (f *DriveFile) IsDir() bool {
	return f.ContentType == "dir"
}

// GetMTime returns the modification time as time.Time
func (f *DriveFile) GetMTime() *time.Time {
	if f.MTime <= 0 {
		return nil
	}
	t := time.Unix(f.MTime, 0)
	return &t
}

// GetATime returns the access time as time.Time
func (f *DriveFile) GetATime() *time.Time {
	if f.ATime <= 0 {
		return nil
	}
	t := time.Unix(f.ATime, 0)
	return &t
}

// DriveLabel represents a label in Drive
type DriveLabel struct {
	ID   string `json:"label_id"`
	Name string `json:"name"`
}

// DriveListResponse is the response from listing Drive files
type DriveListResponse struct {
	Offset int         `json:"offset"`
	Total  int         `json:"total"`
	Items  []DriveFile `json:"items"`
}

// DriveListOptions contains options for listing files in Drive
type DriveListOptions struct {
	Path          string // Folder path
	FileID        int64  // Alternative: use file ID instead of path
	Offset        int
	Limit         int
	SortBy        string // name, time, size, type
	SortDirection string // asc, desc
	FileType      string // dir, file, all
}

// AdvanceSharingInfo represents advanced sharing information for a file
type AdvanceSharingInfo struct {
	SharingLink     string `json:"sharing_link"`
	URL             string `json:"url"`
	ProtectPassword string `json:"protect_password"`
	DueDate         int64  `json:"due_date"`
}

// GetExpiresAt returns the expiration time as *time.Time
func (a *AdvanceSharingInfo) GetExpiresAt() *time.Time {
	if a.DueDate <= 0 {
		return nil
	}
	t := time.Unix(a.DueDate, 0)
	return &t
}

// DriveClient defines the interface for Synology Drive API operations
type DriveClient interface {
	// GetSharedFiles returns files shared with others
	GetSharedFiles(offset, limit int) (*DriveListResponse, error)

	// GetStarredFiles returns starred files
	GetStarredFiles(offset, limit int) (*DriveListResponse, error)

	// GetLabeledFiles returns files with a specific label
	GetLabeledFiles(labelID string, offset, limit int) (*DriveListResponse, error)

	// GetRecentFiles returns recently accessed/modified files
	GetRecentFiles(offset, limit int) (*DriveListResponse, error)

	// GetLabels returns all labels
	GetLabels() ([]DriveLabel, error)

	// ListFiles lists files in a folder
	ListFiles(opts *DriveListOptions) (*DriveListResponse, error)

	// DownloadFile downloads a file
	// Returns: body reader, filename, content length, error
	DownloadFile(fileID int64, path string) (io.ReadCloser, string, int64, error)

	// DownloadFileWithRange downloads a file with byte range support for resume
	DownloadFileWithRange(fileID int64, path string, rangeStart int64) (io.ReadCloser, string, int64, error)

	// GetAdvanceSharing gets advanced sharing info for a file
	GetAdvanceSharing(fileID int64, path string) (*AdvanceSharingInfo, error)
}
