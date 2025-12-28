package vo

import (
	"errors"
	"strconv"
	"strings"
)

// SynoFileID represents a Synology file ID value object.
// This is Synology's unique identifier for files.
type SynoFileID struct {
	value string
}

var (
	ErrEmptySynoFileID   = errors.New("synology file ID cannot be empty")
	ErrInvalidSynoFileID = errors.New("invalid synology file ID format")
)

// NewSynoFileID creates a new SynoFileID value object.
func NewSynoFileID(id string) (SynoFileID, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SynoFileID{}, ErrEmptySynoFileID
	}
	return SynoFileID{value: id}, nil
}

// NewSynoFileIDFromInt creates a SynoFileID from an integer.
func NewSynoFileIDFromInt(id int64) SynoFileID {
	return SynoFileID{value: strconv.FormatInt(id, 10)}
}

// MustSynoFileID creates a new SynoFileID, panicking if invalid.
func MustSynoFileID(id string) SynoFileID {
	sfid, err := NewSynoFileID(id)
	if err != nil {
		panic(err)
	}
	return sfid
}

// EmptySynoFileID returns an empty SynoFileID.
func EmptySynoFileID() SynoFileID {
	return SynoFileID{}
}

// String returns the string representation of the ID.
func (id SynoFileID) String() string {
	return id.value
}

// IsEmpty returns true if the ID is empty.
func (id SynoFileID) IsEmpty() bool {
	return id.value == ""
}

// IsValid returns true if the ID is not empty.
func (id SynoFileID) IsValid() bool {
	return id.value != ""
}

// Equals checks if two IDs are equal.
func (id SynoFileID) Equals(other SynoFileID) bool {
	return id.value == other.value
}

// ToInt64 attempts to convert the ID to int64.
// Returns 0 if conversion fails.
func (id SynoFileID) ToInt64() int64 {
	if id.value == "" {
		return 0
	}
	n, err := strconv.ParseInt(id.value, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
