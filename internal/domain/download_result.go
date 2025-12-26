package domain

// DownloadResult represents the result of a file download operation
type DownloadResult struct {
	// CachePath is the local path where the file was saved
	CachePath string

	// BytesWritten is the total bytes written to disk
	BytesWritten int64

	// Resumed indicates whether the download was resumed from a previous attempt
	Resumed bool

	// ResumedFrom is the byte position from which the download was resumed
	ResumedFrom int64
}
