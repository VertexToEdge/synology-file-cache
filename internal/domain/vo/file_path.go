package vo

import (
	"errors"
	"path/filepath"
	"strings"
)

// FilePath represents a file path value object.
// It normalizes and validates file paths.
type FilePath struct {
	value string
}

var (
	ErrEmptyPath   = errors.New("file path cannot be empty")
	ErrInvalidPath = errors.New("invalid file path")
)

// NewFilePath creates a new FilePath value object.
func NewFilePath(path string) (FilePath, error) {
	if path == "" {
		return FilePath{}, ErrEmptyPath
	}
	// Normalize the path (use forward slashes for consistency)
	normalized := filepath.ToSlash(filepath.Clean(path))
	return FilePath{value: normalized}, nil
}

// MustFilePath creates a new FilePath, panicking if invalid.
// Use only when path is known to be valid.
func MustFilePath(path string) FilePath {
	fp, err := NewFilePath(path)
	if err != nil {
		panic(err)
	}
	return fp
}

// EmptyFilePath returns an empty FilePath.
func EmptyFilePath() FilePath {
	return FilePath{}
}

// String returns the string representation of the path.
func (fp FilePath) String() string {
	return fp.value
}

// IsEmpty returns true if the path is empty.
func (fp FilePath) IsEmpty() bool {
	return fp.value == ""
}

// FileName returns the base name of the file.
func (fp FilePath) FileName() string {
	return filepath.Base(fp.value)
}

// Extension returns the file extension (including the dot).
func (fp FilePath) Extension() string {
	return filepath.Ext(fp.value)
}

// Dir returns the directory part of the path.
func (fp FilePath) Dir() FilePath {
	dir := filepath.Dir(fp.value)
	return FilePath{value: filepath.ToSlash(dir)}
}

// ToCachePath converts a Synology path to a local cache path.
func (fp FilePath) ToCachePath(rootDir string) FilePath {
	if fp.value == "" {
		return FilePath{}
	}
	cachePath := filepath.Join(rootDir, fp.value)
	return FilePath{value: filepath.ToSlash(cachePath)}
}

// ToTempPath returns the temporary download path.
func (fp FilePath) ToTempPath() FilePath {
	if fp.value == "" {
		return FilePath{}
	}
	return FilePath{value: fp.value + ".downloading"}
}

// Join appends path elements to the current path.
func (fp FilePath) Join(elem ...string) FilePath {
	parts := append([]string{fp.value}, elem...)
	joined := filepath.Join(parts...)
	return FilePath{value: filepath.ToSlash(joined)}
}

// HasPrefix checks if the path starts with the given prefix.
func (fp FilePath) HasPrefix(prefix string) bool {
	return strings.HasPrefix(fp.value, prefix)
}

// Equals checks if two paths are equal.
func (fp FilePath) Equals(other FilePath) bool {
	return fp.value == other.value
}
