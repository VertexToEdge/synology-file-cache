package domain

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSkippableError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		context string
		want    string
	}{
		{
			name:    "with context and error",
			err:     errors.New("underlying error"),
			context: "processing file",
			want:    "processing file: underlying error",
		},
		{
			name:    "with context only",
			err:     nil,
			context: "file already cached",
			want:    "file already cached",
		},
		{
			name:    "with error only",
			err:     errors.New("underlying error"),
			context: "",
			want:    "underlying error",
		},
		{
			name:    "empty",
			err:     nil,
			context: "",
			want:    "skippable error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			se := NewSkippableError(tt.err, tt.context)
			if got := se.Error(); got != tt.want {
				t.Errorf("Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSkippableError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	se := NewSkippableError(underlying, "context")

	if got := se.Unwrap(); got != underlying {
		t.Errorf("Unwrap() = %v, want %v", got, underlying)
	}

	// Test with nil error
	seNil := NewSkippableError(nil, "context")
	if got := seNil.Unwrap(); got != nil {
		t.Errorf("Unwrap() with nil = %v, want nil", got)
	}
}

func TestIsSkippable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "skippable error",
			err:  NewSkippableError(errors.New("err"), "context"),
			want: true,
		},
		{
			name: "wrapped skippable error",
			err:  fmt.Errorf("wrapped: %w", NewSkippableError(errors.New("err"), "context")),
			want: true,
		},
		{
			name: "regular error",
			err:  errors.New("regular error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "predefined skippable error",
			err:  ErrSkipFileNotFound,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSkippable(tt.err); got != tt.want {
				t.Errorf("IsSkippable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryableError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "with underlying error",
			err:  errors.New("connection timeout"),
			want: "connection timeout",
		},
		{
			name: "nil error",
			err:  nil,
			want: "retryable error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := NewRetryableError(tt.err, time.Second)
			if got := re.Error(); got != tt.want {
				t.Errorf("Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryableError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	re := NewRetryableError(underlying, time.Second)

	if got := re.Unwrap(); got != underlying {
		t.Errorf("Unwrap() = %v, want %v", got, underlying)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "retryable error",
			err:  NewRetryableError(errors.New("err"), time.Second),
			want: true,
		},
		{
			name: "wrapped retryable error",
			err:  fmt.Errorf("wrapped: %w", NewRetryableError(errors.New("err"), time.Second)),
			want: true,
		},
		{
			name: "regular error",
			err:  errors.New("regular error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "skippable error is not retryable",
			err:  NewSkippableError(errors.New("err"), "context"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRetryAfter(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantDuration time.Duration
		wantOk       bool
	}{
		{
			name:         "retryable error",
			err:          NewRetryableError(errors.New("err"), 5*time.Minute),
			wantDuration: 5 * time.Minute,
			wantOk:       true,
		},
		{
			name:         "wrapped retryable error",
			err:          fmt.Errorf("wrapped: %w", NewRetryableError(errors.New("err"), 30*time.Second)),
			wantDuration: 30 * time.Second,
			wantOk:       true,
		},
		{
			name:         "regular error",
			err:          errors.New("regular error"),
			wantDuration: 0,
			wantOk:       false,
		},
		{
			name:         "nil error",
			err:          nil,
			wantDuration: 0,
			wantOk:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration, ok := GetRetryAfter(tt.err)
			if duration != tt.wantDuration || ok != tt.wantOk {
				t.Errorf("GetRetryAfter() = (%v, %v), want (%v, %v)",
					duration, ok, tt.wantDuration, tt.wantOk)
			}
		})
	}
}

func TestErrorsAsUnwrap(t *testing.T) {
	// Test that errors.Is works with SkippableError wrapping domain errors
	se := NewSkippableError(ErrNotFound, "context")

	if !errors.Is(se, ErrNotFound) {
		t.Error("SkippableError should unwrap to ErrNotFound")
	}

	// Test that errors.Is works with RetryableError
	re := NewRetryableError(ErrInsufficientSpace, time.Second)

	if !errors.Is(re, ErrInsufficientSpace) {
		t.Error("RetryableError should unwrap to ErrInsufficientSpace")
	}
}

func TestPredefinedSkippableErrors(t *testing.T) {
	// Test that predefined errors work correctly
	if !IsSkippable(ErrSkipFileNotFound) {
		t.Error("ErrSkipFileNotFound should be skippable")
	}

	if !errors.Is(ErrSkipFileNotFound, ErrNotFound) {
		t.Error("ErrSkipFileNotFound should unwrap to ErrNotFound")
	}

	if !IsSkippable(ErrSkipAlreadyCached) {
		t.Error("ErrSkipAlreadyCached should be skippable")
	}

	if !IsSkippable(ErrSkipTaskExists) {
		t.Error("ErrSkipTaskExists should be skippable")
	}

	if !errors.Is(ErrSkipTaskExists, ErrAlreadyExists) {
		t.Error("ErrSkipTaskExists should unwrap to ErrAlreadyExists")
	}
}
