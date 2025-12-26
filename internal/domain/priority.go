package domain

// Priority levels for cache
// Lower number = higher priority
const (
	PriorityShared         = 1 // Files shared with others (most important)
	PriorityStarred        = 2 // Starred files and labeled files
	PriorityRecentModified = 3 // Recently modified files
	PriorityRecentAccessed = 4 // Recently accessed files
	PriorityDefault        = 5 // Default priority
)

// PriorityName returns a human-readable name for the priority level
func PriorityName(priority int) string {
	switch priority {
	case PriorityShared:
		return "shared"
	case PriorityStarred:
		return "starred"
	case PriorityRecentModified:
		return "recent_modified"
	case PriorityRecentAccessed:
		return "recent_accessed"
	case PriorityDefault:
		return "default"
	default:
		return "unknown"
	}
}
