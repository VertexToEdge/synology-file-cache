package domain

import (
	"time"
)

// Share represents a shared link for a file
type Share struct {
	ID          int64
	SynoShareID string
	Token       string // Token from permanent_link (e.g., 167e18n3x0hcXGDIrZV45Gp5uf66gpac)
	SharingLink string // sharing_link from AdvanceSharing API
	URL         string // Full URL from AdvanceSharing API
	FileID      int64
	Password    string // Password if set (empty string if no password)
	ExpiresAt   *time.Time
	CreatedAt   time.Time
	Revoked     bool
}

// HasPassword returns true if the share is password protected
func (s *Share) HasPassword() bool {
	return s.Password != ""
}

// IsExpired returns true if the share has expired
func (s *Share) IsExpired() bool {
	if s.ExpiresAt == nil {
		return false
	}
	return s.ExpiresAt.Before(time.Now())
}

// IsValid returns true if the share is valid (not revoked and not expired)
func (s *Share) IsValid() bool {
	return !s.Revoked && !s.IsExpired()
}
