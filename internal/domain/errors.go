package domain

import "errors"

// Common domain errors
var (
	ErrNotFound       = errors.New("not found")
	ErrAlreadyExists  = errors.New("already exists")
	ErrInvalidInput   = errors.New("invalid input")
	ErrShareRevoked   = errors.New("share has been revoked")
	ErrShareExpired   = errors.New("share has expired")
	ErrFileNotCached  = errors.New("file not cached")
	ErrInsufficientSpace = errors.New("insufficient space")
)
