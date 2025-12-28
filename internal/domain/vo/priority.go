package vo

// Priority represents a caching priority value object.
// Lower values indicate higher priority.
type Priority struct {
	level int
}

// Priority constants (lower = higher priority)
var (
	PriorityShared         = Priority{level: 1} // Shared files (highest priority)
	PriorityStarred        = Priority{level: 2} // Starred + Labeled files
	PriorityRecentModified = Priority{level: 3} // Recently modified files
	PriorityRecentAccessed = Priority{level: 4} // Recently accessed files
	PriorityDefault        = Priority{level: 5} // Default priority (lowest)
)

// NewPriority creates a Priority from an integer level.
// Valid levels are 1-5, invalid values are clamped.
func NewPriority(level int) Priority {
	if level < 1 {
		level = 1
	}
	if level > 5 {
		level = 5
	}
	return Priority{level: level}
}

// Value returns the numeric priority level.
func (p Priority) Value() int {
	return p.level
}

// Name returns a human-readable name for this priority.
func (p Priority) Name() string {
	switch p.level {
	case 1:
		return "Shared"
	case 2:
		return "Starred"
	case 3:
		return "RecentModified"
	case 4:
		return "RecentAccessed"
	case 5:
		return "Default"
	default:
		return "Unknown"
	}
}

// HigherThan returns true if this priority is higher (lower number) than other.
func (p Priority) HigherThan(other Priority) bool {
	return p.level < other.level
}

// LowerThan returns true if this priority is lower (higher number) than other.
func (p Priority) LowerThan(other Priority) bool {
	return p.level > other.level
}

// Equals returns true if both priorities are equal.
func (p Priority) Equals(other Priority) bool {
	return p.level == other.level
}

// ShouldUpdate returns true if this priority should update the existing priority.
// Only updates to higher priority (lower number).
func (p Priority) ShouldUpdate(existing Priority) bool {
	return p.HigherThan(existing)
}

// IsDefault returns true if this is the default priority.
func (p Priority) IsDefault() bool {
	return p.level == PriorityDefault.level
}

// IsHighPriority returns true if priority level is 1 or 2 (shared or starred).
func (p Priority) IsHighPriority() bool {
	return p.level <= 2
}
