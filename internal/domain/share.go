package domain

import (
	"crypto/subtle"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/domain/vo"
)

// SharePassword encapsulates password validation logic
type SharePassword struct {
	value string
}

// NewSharePassword creates a new SharePassword
func NewSharePassword(password string) SharePassword {
	return SharePassword{value: password}
}

// IsSet returns true if a password is set
func (sp SharePassword) IsSet() bool {
	return sp.value != ""
}

// Verify checks if the input password matches
// Uses constant-time comparison to prevent timing attacks
func (sp SharePassword) Verify(input string) bool {
	if !sp.IsSet() {
		return true // No password required
	}
	return subtle.ConstantTimeCompare([]byte(sp.value), []byte(input)) == 1
}

// String returns the password value (use with caution)
func (sp SharePassword) String() string {
	return sp.value
}

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

// GetToken returns the token as a value object
func (s *Share) GetToken() vo.ShareToken {
	if s.Token == "" {
		return vo.EmptyShareToken()
	}
	st, _ := vo.NewShareToken(s.Token)
	return st
}

// GetPassword returns the password as a SharePassword value object
func (s *Share) GetPassword() SharePassword {
	return NewSharePassword(s.Password)
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

// ValidateAccess validates if the share can be accessed
// Returns nil if access is allowed, or a domain error otherwise
func (s *Share) ValidateAccess() error {
	if s.Revoked {
		return ErrShareRevoked
	}
	if s.IsExpired() {
		return ErrShareExpired
	}
	return nil
}

// VerifyPassword verifies the input password against the share password
// Returns nil if password is correct or not required, or ErrInvalidPassword otherwise
func (s *Share) VerifyPassword(input string) error {
	if !s.GetPassword().Verify(input) {
		return ErrInvalidPassword
	}
	return nil
}

// FullValidation performs complete validation including password check
func (s *Share) FullValidation(password string) error {
	if err := s.ValidateAccess(); err != nil {
		return err
	}
	return s.VerifyPassword(password)
}

// Revoke marks the share as revoked
func (s *Share) Revoke() {
	s.Revoked = true
}

// SetExpiration sets the expiration time
func (s *Share) SetExpiration(expiresAt *time.Time) {
	s.ExpiresAt = expiresAt
}

// RemainingTime returns the time until expiration, or 0 if no expiration
func (s *Share) RemainingTime() time.Duration {
	if s.ExpiresAt == nil {
		return 0
	}
	remaining := time.Until(*s.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}
