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
	ProfileSmall  = "small"
	ProfileMedium = "medium"
	ProfileLarge  = "large"
)

// ResourceProfileConfig represents a resource profile configuration
type ResourceProfileConfig struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Memory      string `json:"memory"`
	MemoryBytes int64  `json:"memory_bytes"`
	CPU         int    `json:"cpu"`
	IsDefault   bool   `json:"is_default,omitempty"`
}

// ResourceProfiles contains all available resource profile configurations
var ResourceProfiles = []ResourceProfileConfig{
	{
		Value:       ProfileSmall,
		Label:       "Small (8GB Memory, 1 CPU)",
		Description: "Suitable for small trace files (< 5MB)",
		Memory:      "8Gi",
		MemoryBytes: 8 * 1024 * 1024 * 1024,
		CPU:         1,
	},
	{
		Value:       ProfileMedium,
		Label:       "Medium (16GB Memory, 2 CPU)",
		Description: "Recommended (5-20MB)",
		Memory:      "16Gi",
		MemoryBytes: 16 * 1024 * 1024 * 1024,
		CPU:         2,
		IsDefault:   true,
	},
	{
		Value:       ProfileLarge,
		Label:       "Large (32GB Memory, 4 CPU)",
		Description: "Suitable for large trace files (> 20MB)",
		Memory:      "32Gi",
		MemoryBytes: 32 * 1024 * 1024 * 1024,
		CPU:         4,
	},
}

// GetResourceProfile returns the resource profile config by value
func GetResourceProfile(value string) *ResourceProfileConfig {
	for _, p := range ResourceProfiles {
		if p.Value == value {
			return &p
		}
	}
	return nil
}

// Default values
const (
	// DefaultSessionTTL is the default session time-to-live
	DefaultSessionTTL = 1 * time.Hour

	// DefaultPodNamespace is the default namespace for TraceLens pods
	// Pods are created in the management cluster
	DefaultPodNamespace = "primus-lens"

	// DefaultPodPort is the default port for Streamlit UI
	DefaultPodPort = 8501

	// MaxSessionTTL is the maximum allowed session TTL
	MaxSessionTTL = 4 * time.Hour

	// SessionIDPrefix is the prefix for session IDs
	SessionIDPrefix = "tls"

	// DefaultTraceLensImage is the default container image for TraceLens pods
	DefaultTraceLensImage = "harbor.tw325.primus-safe.amd.com/primussafe/tracelens:latest"
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

