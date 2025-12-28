package vo

import (
	"errors"
	"strings"
)

// ShareToken represents a Synology share token value object.
// This is the permanent_link token used for file sharing.
type ShareToken struct {
	value string
}

var (
	ErrEmptyToken   = errors.New("share token cannot be empty")
	ErrInvalidToken = errors.New("invalid share token format")
)

// NewShareToken creates a new ShareToken value object.
func NewShareToken(token string) (ShareToken, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return ShareToken{}, ErrEmptyToken
	}
	return ShareToken{value: token}, nil
}

// MustShareToken creates a new ShareToken, panicking if invalid.
func MustShareToken(token string) ShareToken {
	st, err := NewShareToken(token)
	if err != nil {
		panic(err)
	}
	return st
}

// EmptyShareToken returns an empty ShareToken.
func EmptyShareToken() ShareToken {
	return ShareToken{}
}

// String returns the string representation of the token.
func (st ShareToken) String() string {
	return st.value
}

// IsEmpty returns true if the token is empty.
func (st ShareToken) IsEmpty() bool {
	return st.value == ""
}

// IsValid returns true if the token is not empty.
func (st ShareToken) IsValid() bool {
	return st.value != ""
}

// Equals checks if two tokens are equal.
func (st ShareToken) Equals(other ShareToken) bool {
	return st.value == other.value
}

// Masked returns a masked version of the token for logging.
// Shows first 4 and last 4 characters with asterisks in between.
func (st ShareToken) Masked() string {
	if len(st.value) <= 8 {
		return strings.Repeat("*", len(st.value))
	}
	return st.value[:4] + "****" + st.value[len(st.value)-4:]
}
