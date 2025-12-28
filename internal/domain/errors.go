package domain

import (
	"errors"
	"time"
)

// Common domain errors
var (
	ErrNotFound          = errors.New("not found")
	ErrAlreadyExists     = errors.New("already exists")
	ErrInvalidInput      = errors.New("invalid input")
	ErrShareRevoked      = errors.New("share has been revoked")
	ErrShareExpired      = errors.New("share has expired")
	ErrFileNotCached     = errors.New("file not cached")
	ErrInsufficientSpace = errors.New("insufficient space")

	// File domain errors
	ErrFileTooLarge      = errors.New("file exceeds maximum cache size")
	ErrFileAlreadyCached = errors.New("file is already cached")
	ErrInvalidFileState  = errors.New("invalid file state")

	// Share domain errors
	ErrInvalidPassword = errors.New("invalid share password")
	ErrShareNotFound   = errors.New("share not found")

	// Download task domain errors
	ErrTaskNotFound         = errors.New("download task not found")
	ErrTaskAlreadyExists    = errors.New("download task already exists for this file")
	ErrTaskAlreadyClaimed   = errors.New("task is already claimed by another worker")
	ErrInvalidTaskState     = errors.New("invalid task state")
	ErrInvalidStateTransition = errors.New("invalid state transition")

	// Aggregate errors
	ErrNilFile      = errors.New("file cannot be nil")
	ErrNilAggregate = errors.New("aggregate cannot be nil")
)

// SkippableError represents an error that can be logged and skipped.
// Processing can continue with the next item when this error occurs.
type SkippableError struct {
	Err     error
	Context string
}

// Error returns the error message
func (e *SkippableError) Error() string {
	if e.Context != "" {
		if e.Err != nil {
			return e.Context + ": " + e.Err.Error()
		}
		return e.Context
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "skippable error"
}

// Unwrap returns the underlying error
func (e *SkippableError) Unwrap() error {
	return e.Err
}

// NewSkippableError creates a new skippable error
func NewSkippableError(err error, context string) *SkippableError {
	return &SkippableError{Err: err, Context: context}
}

// IsSkippable returns true if the error can be skipped
func IsSkippable(err error) bool {
	var se *SkippableError
	return errors.As(err, &se)
}

// RetryableError represents an error that should trigger a retry.
type RetryableError struct {
	Err        error
	RetryAfter time.Duration
}

// Error returns the error message
func (e *RetryableError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "retryable error"
}

// Unwrap returns the underlying error
func (e *RetryableError) Unwrap() error {
	return e.Err
}

// NewRetryableError creates a new retryable error
func NewRetryableError(err error, retryAfter time.Duration) *RetryableError {
	return &RetryableError{Err: err, RetryAfter: retryAfter}
}

// IsRetryable returns true if the error should be retried
func IsRetryable(err error) bool {
	var re *RetryableError
	return errors.As(err, &re)
}

// GetRetryAfter returns the retry duration if the error is retryable
func GetRetryAfter(err error) (time.Duration, bool) {
	var re *RetryableError
	if errors.As(err, &re) {
		return re.RetryAfter, true
	}
	return 0, false
}

// Common skippable errors for convenience
var (
	ErrSkipFileNotFound  = NewSkippableError(ErrNotFound, "file not found")
	ErrSkipAlreadyCached = NewSkippableError(nil, "file already cached")
	ErrSkipTaskExists    = NewSkippableError(ErrAlreadyExists, "task already exists")
)
