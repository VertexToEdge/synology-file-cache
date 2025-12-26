package synology

import (
	"encoding/json"
)

// APIEndpoint contains API path and version information
type APIEndpoint struct {
	Path       string `json:"path"`
	MinVersion int    `json:"minVersion"`
	MaxVersion int    `json:"maxVersion"`
}

// Response is the base response structure from Synology API
type Response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *ErrorInfo      `json:"error,omitempty"`
}

// ErrorInfo contains error details
type ErrorInfo struct {
	Code   int             `json:"code"`
	Errors json.RawMessage `json:"errors,omitempty"`
}

// Common error codes
const (
	ErrUnknown         = 100
	ErrInvalidParam    = 101
	ErrAPINotExists    = 102
	ErrMethodNotExists = 103
	ErrVersionNotSupport = 104
	ErrNoPermission    = 105
	ErrSessionTimeout  = 106
	ErrDuplicateLogin  = 107
	ErrSIDNotFound     = 119
)

// APIError represents an error from the Synology API
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return e.Message
}

// IsSessionError returns true if the error indicates session issues
func (e *APIError) IsSessionError() bool {
	return e.Code == ErrNoPermission || e.Code == ErrSessionTimeout || e.Code == ErrSIDNotFound
}

// errorMessages maps error codes to human-readable messages
var errorMessages = map[int]string{
	ErrUnknown:         "unknown error",
	ErrInvalidParam:    "invalid parameter",
	ErrAPINotExists:    "api does not exist",
	ErrMethodNotExists: "method does not exist",
	ErrVersionNotSupport: "version not supported",
	ErrNoPermission:    "no permission",
	ErrSessionTimeout:  "session timeout",
	ErrDuplicateLogin:  "duplicate login",
	ErrSIDNotFound:     "sid not found",
}

// GetErrorMessage returns a human-readable message for an error code
func GetErrorMessage(code int) string {
	if msg, ok := errorMessages[code]; ok {
		return msg
	}
	return "error code " + string(rune(code))
}

// Drive API names
const (
	APIDriveFiles          = "SYNO.SynologyDrive.Files"
	APIDriveNode           = "SYNO.SynologyDrive.Node"
	APIDriveDownload       = "SYNO.SynologyDrive.Node.Download"
	APIDriveSharing        = "SYNO.SynologyDrive.Sharing"
	APIDriveShare          = "SYNO.SynologyDrive.Share"
	APIDriveAdvanceSharing = "SYNO.SynologyDrive.AdvanceSharing"
	APIDriveLabels         = "SYNO.SynologyDrive.Labels"
	APIDriveTeamFolder     = "SYNO.SynologyDrive.TeamFolders"
)

const (
	apiInfoPath = "query.cgi"
	authPath    = "auth.cgi"
	sessionName = "FileStation"
)
