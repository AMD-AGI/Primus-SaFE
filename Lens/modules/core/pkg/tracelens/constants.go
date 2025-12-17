package tracelens

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

// Resource Profile constants
const (
	ProfileSmall  = "small"  // 2GB RAM
	ProfileMedium = "medium" // 4GB RAM
	ProfileLarge  = "large"  // 8GB RAM
)

// Default values
const (
	// DefaultSessionTTL is the default session time-to-live
	DefaultSessionTTL = 1 * time.Hour

	// DefaultPodNamespace is the default namespace for TraceLens pods
	DefaultPodNamespace = "primus-lens"

	// DefaultPodPort is the default port for Streamlit UI
	DefaultPodPort = 8501

	// MaxSessionTTL is the maximum allowed session TTL
	MaxSessionTTL = 4 * time.Hour

	// SessionIDPrefix is the prefix for session IDs
	SessionIDPrefix = "tls"
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

// ValidResourceProfiles returns all valid resource profiles
func ValidResourceProfiles() []string {
	return []string{
		ProfileSmall,
		ProfileMedium,
		ProfileLarge,
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

// IsValidResourceProfile checks if a resource profile is valid
func IsValidResourceProfile(profile string) bool {
	for _, p := range ValidResourceProfiles() {
		if p == profile {
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

