package tracelens

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidStatuses(t *testing.T) {
	statuses := ValidStatuses()

	assert.NotEmpty(t, statuses)
	assert.Contains(t, statuses, StatusPending)
	assert.Contains(t, statuses, StatusCreating)
	assert.Contains(t, statuses, StatusInitializing)
	assert.Contains(t, statuses, StatusReady)
	assert.Contains(t, statuses, StatusFailed)
	assert.Contains(t, statuses, StatusExpired)
	assert.Contains(t, statuses, StatusDeleted)
	assert.Len(t, statuses, 7)
}

func TestActiveStatuses(t *testing.T) {
	statuses := ActiveStatuses()

	assert.NotEmpty(t, statuses)
	assert.Contains(t, statuses, StatusPending)
	assert.Contains(t, statuses, StatusCreating)
	assert.Contains(t, statuses, StatusInitializing)
	assert.Contains(t, statuses, StatusReady)
	// Failed, expired, deleted should not be active
	assert.NotContains(t, statuses, StatusFailed)
	assert.NotContains(t, statuses, StatusExpired)
	assert.NotContains(t, statuses, StatusDeleted)
	assert.Len(t, statuses, 4)
}

func TestValidResourceProfiles(t *testing.T) {
	profiles := ValidResourceProfiles()

	assert.NotEmpty(t, profiles)
	assert.Contains(t, profiles, ProfileSmall)
	assert.Contains(t, profiles, ProfileMedium)
	assert.Contains(t, profiles, ProfileLarge)
	assert.Len(t, profiles, 3)
}

func TestIsValidStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{"valid pending", StatusPending, true},
		{"valid creating", StatusCreating, true},
		{"valid initializing", StatusInitializing, true},
		{"valid ready", StatusReady, true},
		{"valid failed", StatusFailed, true},
		{"valid expired", StatusExpired, true},
		{"valid deleted", StatusDeleted, true},
		{"invalid empty", "", false},
		{"invalid random", "random_status", false},
		{"invalid uppercase", "PENDING", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidStatus(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidResourceProfile(t *testing.T) {
	tests := []struct {
		name     string
		profile  string
		expected bool
	}{
		{"valid small", ProfileSmall, true},
		{"valid medium", ProfileMedium, true},
		{"valid large", ProfileLarge, true},
		{"invalid empty", "", false},
		{"invalid random", "extra_large", false},
		{"invalid uppercase", "SMALL", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidResourceProfile(tt.profile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsActiveStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{"active pending", StatusPending, true},
		{"active creating", StatusCreating, true},
		{"active initializing", StatusInitializing, true},
		{"active ready", StatusReady, true},
		{"not active failed", StatusFailed, false},
		{"not active expired", StatusExpired, false},
		{"not active deleted", StatusDeleted, false},
		{"not active empty", "", false},
		{"not active random", "random", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsActiveStatus(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConstantValues(t *testing.T) {
	// Test status constants
	assert.Equal(t, "pending", StatusPending)
	assert.Equal(t, "creating", StatusCreating)
	assert.Equal(t, "initializing", StatusInitializing)
	assert.Equal(t, "ready", StatusReady)
	assert.Equal(t, "failed", StatusFailed)
	assert.Equal(t, "expired", StatusExpired)
	assert.Equal(t, "deleted", StatusDeleted)

	// Test profile constants
	assert.Equal(t, "small", ProfileSmall)
	assert.Equal(t, "medium", ProfileMedium)
	assert.Equal(t, "large", ProfileLarge)

	// Test default values
	assert.Equal(t, 1*time.Hour, DefaultSessionTTL)
	assert.Equal(t, "primus-lens", DefaultPodNamespace)
	assert.Equal(t, 8501, DefaultPodPort)
	assert.Equal(t, 4*time.Hour, MaxSessionTTL)
	assert.Equal(t, "tls", SessionIDPrefix)
	assert.NotEmpty(t, DefaultTraceLensImage)
}

