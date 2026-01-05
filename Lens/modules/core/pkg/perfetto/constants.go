package perfetto

import "time"

// Session Status constants
const (
	StatusPending      = "pending"
	StatusCreating     = "creating"
	StatusInitializing = "initializing"
	StatusReady        = "ready"
	StatusFailed       = "failed"
	StatusExpired      = "expired"
	StatusDeleted      = "deleted"
)

// Default values
const (
	// DefaultSessionTTL is the default session time-to-live
	DefaultSessionTTL = 30 * time.Minute

	// DefaultPodNamespace is the default namespace for Perfetto pods
	DefaultPodNamespace = "primus-lens"

	// DefaultPodPort is the default port for Perfetto UI (nginx)
	DefaultPodPort = 8080

	// MaxSessionTTL is the maximum allowed session TTL
	MaxSessionTTL = 2 * time.Hour

	// SessionIDPrefix is the prefix for session IDs
	SessionIDPrefix = "pft"

	// DefaultPerfettoImage is the default container image for Perfetto pods
	DefaultPerfettoImage = "harbor.tw325.primus-safe.amd.com/primussafe/perfetto-viewer"

	// PodMemoryLimit is the memory limit for Perfetto pods (lightweight)
	PodMemoryLimit = "512Mi"

	// PodCPULimit is the CPU limit for Perfetto pods
	PodCPULimit = "500m"
)

// ValidStatuses returns all valid session statuses
func ValidStatuses() []string {
	return []string{
		StatusPending,
		StatusCreating,
		StatusInitializing,
		StatusReady,
		StatusFailed,
		StatusExpired,
		StatusDeleted,
	}
}

// ActiveStatuses returns statuses considered as "active"
func ActiveStatuses() []string {
	return []string{
		StatusPending,
		StatusCreating,
		StatusInitializing,
		StatusReady,
	}
}

// IsValidStatus checks if a status is valid
func IsValidStatus(status string) bool {
	for _, s := range ValidStatuses() {
		if s == status {
			return true
		}
	}
	return false
}

// IsActiveStatus checks if a status is considered active
func IsActiveStatus(status string) bool {
	for _, s := range ActiveStatuses() {
		if s == status {
			return true
		}
	}
	return false
}

