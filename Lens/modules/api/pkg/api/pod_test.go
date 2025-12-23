package api

import (
	"testing"
)

func TestGetStatusFromPhase(t *testing.T) {
	tests := []struct {
		name     string
		phase    string
		running  bool
		expected string
	}{
		{
			name:     "Running pod",
			phase:    "Running",
			running:  true,
			expected: "Running",
		},
		{
			name:     "Pending pod",
			phase:    "Pending",
			running:  false,
			expected: "Pending",
		},
		{
			name:     "Succeeded pod",
			phase:    "Succeeded",
			running:  false,
			expected: "Succeeded",
		},
		{
			name:     "Failed pod",
			phase:    "Failed",
			running:  false,
			expected: "Failed",
		},
		{
			name:     "Unknown phase",
			phase:    "Unknown",
			running:  false,
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStatusFromPhase(tt.phase, tt.running)
			if result != tt.expected {
				t.Errorf("getStatusFromPhase(%s, %v) = %s; want %s",
					tt.phase, tt.running, result, tt.expected)
			}
		})
	}
}

func TestPodStatsQueryParamsDefaults(t *testing.T) {
	params := PodStatsQueryParams{
		Cluster: "test-cluster",
	}

	if params.Cluster != "test-cluster" {
		t.Errorf("Expected cluster to be test-cluster, got %s", params.Cluster)
	}
}
