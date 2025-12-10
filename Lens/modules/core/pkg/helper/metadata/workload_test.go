package metadata

import (
	"testing"
)

func TestGetWorkloadStatusColor(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{
			name:     "Running status should return green",
			status:   WorkloadStatusRunning,
			expected: "green",
		},
		{
			name:     "Pending status should return yellow",
			status:   WorkloadStatusPending,
			expected: "yellow",
		},
		{
			name:     "Done status should return blue",
			status:   WorkloadStatusDone,
			expected: "blue",
		},
		{
			name:     "Deleted status should return gray",
			status:   WorkloadStatusDeleted,
			expected: "gray",
		},
		{
			name:     "Failed status should return red",
			status:   WorkloadStatusFailed,
			expected: "red",
		},
		{
			name:     "Unknown status should return empty string",
			status:   "Unknown",
			expected: "",
		},
		{
			name:     "Empty status should return empty string",
			status:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetWorkloadStatusColor(tt.status)
			if result != tt.expected {
				t.Errorf("GetWorkloadStatusColor(%q) = %q, want %q", tt.status, result, tt.expected)
			}
		})
	}
}

func TestWorkloadStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "WorkloadStatusRunning constant value",
			constant: WorkloadStatusRunning,
			expected: "Running",
		},
		{
			name:     "WorkloadStatusPending constant value",
			constant: WorkloadStatusPending,
			expected: "Pending",
		},
		{
			name:     "WorkloadStatusDone constant value",
			constant: WorkloadStatusDone,
			expected: "Done",
		},
		{
			name:     "WorkloadStatusDeleted constant value",
			constant: WorkloadStatusDeleted,
			expected: "Deleted",
		},
		{
			name:     "WorkloadStatusFailed constant value",
			constant: WorkloadStatusFailed,
			expected: "Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("Constant value = %q, want %q", tt.constant, tt.expected)
			}
		})
	}
}

func TestWorkloadStatusColorMapIntegrity(t *testing.T) {
	expectedStatuses := []string{
		WorkloadStatusRunning,
		WorkloadStatusPending,
		WorkloadStatusDone,
		WorkloadStatusDeleted,
		WorkloadStatusFailed,
	}

	for _, status := range expectedStatuses {
		t.Run("Status "+status+" should have color mapping", func(t *testing.T) {
			color := workloadStatusColorMap[status]
			if color == "" {
				t.Errorf("Status %q does not have a color mapping", status)
			}
		})
	}
}

func TestWorkloadStatusColorMapCount(t *testing.T) {
	expectedCount := 5
	actualCount := len(workloadStatusColorMap)
	if actualCount != expectedCount {
		t.Errorf("workloadStatusColorMap has %d entries, expected %d", actualCount, expectedCount)
	}
}

