package vo

import (
	"errors"
	"fmt"
)

// FileSize represents a file size value object.
// It provides type-safe operations and human-readable formatting.
type FileSize struct {
	bytes int64
}

const (
	KB int64 = 1024
	MB int64 = 1024 * KB
	GB int64 = 1024 * MB
	TB int64 = 1024 * GB
)

var (
	ErrNegativeSize = errors.New("file size cannot be negative")
)

// NewFileSize creates a new FileSize value object.
func NewFileSize(bytes int64) (FileSize, error) {
	if bytes < 0 {
		return FileSize{}, ErrNegativeSize
	}
	return FileSize{bytes: bytes}, nil
}

// MustFileSize creates a new FileSize, panicking if invalid.
func MustFileSize(bytes int64) FileSize {
	fs, err := NewFileSize(bytes)
	if err != nil {
		panic(err)
	}
	return fs
}

// ZeroSize returns a zero FileSize.
func ZeroSize() FileSize {
	return FileSize{bytes: 0}
}

// FileSizeFromGB creates a FileSize from gigabytes.
func FileSizeFromGB(gb float64) FileSize {
	return FileSize{bytes: int64(gb * float64(GB))}
}

// FileSizeFromMB creates a FileSize from megabytes.
func FileSizeFromMB(mb float64) FileSize {
	return FileSize{bytes: int64(mb * float64(MB))}
}

// Bytes returns the size in bytes.
func (fs FileSize) Bytes() int64 {
	return fs.bytes
}

// KB returns the size in kilobytes.
func (fs FileSize) KB() float64 {
	return float64(fs.bytes) / float64(KB)
}

// MB returns the size in megabytes.
func (fs FileSize) MB() float64 {
	return float64(fs.bytes) / float64(MB)
}

// GB returns the size in gigabytes.
func (fs FileSize) GB() float64 {
	return float64(fs.bytes) / float64(GB)
}

// IsZero returns true if the size is zero.
func (fs FileSize) IsZero() bool {
	return fs.bytes == 0
}

// ExceedsLimit checks if this size exceeds the given limit.
func (fs FileSize) ExceedsLimit(limit FileSize) bool {
	return fs.bytes > limit.bytes
}

// Add returns a new FileSize with the given size added.
func (fs FileSize) Add(other FileSize) FileSize {
	return FileSize{bytes: fs.bytes + other.bytes}
}

// Subtract returns a new FileSize with the given size subtracted.
// Returns zero if result would be negative.
func (fs FileSize) Subtract(other FileSize) FileSize {
	result := fs.bytes - other.bytes
	if result < 0 {
		return ZeroSize()
	}
	return FileSize{bytes: result}
}

// GreaterThan returns true if this size is greater than other.
func (fs FileSize) GreaterThan(other FileSize) bool {
	return fs.bytes > other.bytes
}

// LessThan returns true if this size is less than other.
func (fs FileSize) LessThan(other FileSize) bool {
	return fs.bytes < other.bytes
}

// Equals returns true if both sizes are equal.
func (fs FileSize) Equals(other FileSize) bool {
	return fs.bytes == other.bytes
}

// String returns a human-readable string representation.
func (fs FileSize) String() string {
	bytes := fs.bytes
	if bytes < KB {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < MB {
		return fmt.Sprintf("%.2f KB", fs.KB())
	}
	if bytes < GB {
		return fmt.Sprintf("%.2f MB", fs.MB())
	}
	if bytes < TB {
		return fmt.Sprintf("%.2f GB", fs.GB())
	}
	return fmt.Sprintf("%.2f TB", float64(bytes)/float64(TB))
}
