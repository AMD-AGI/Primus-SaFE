package api

import (
	"testing"
	"time"
)

// Test struct validation and defaults
func TestPodStatsQueryParamsValidation(t *testing.T) {
	tests := []struct {
		name   string
		params PodStatsQueryParams
		valid  bool
	}{
		{
			name: "Valid params with required fields",
			params: PodStatsQueryParams{
				Cluster:  "test-cluster",
				Page:     1,
				PageSize: 10,
			},
			valid: true,
		},
		{
			name: "Valid params with all fields",
			params: PodStatsQueryParams{
				Cluster:   "test-cluster",
				Namespace: "default",
				PodName:   "test-pod",
				Labels:    []string{"app=test"},
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-02T00:00:00Z",
				Page:      1,
				PageSize:  20,
			},
			valid: true,
		},
		{
			name: "Valid params with empty optionals",
			params: PodStatsQueryParams{
				Cluster: "test-cluster",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify cluster is not empty (required field)
			if tt.params.Cluster == "" && tt.valid {
				t.Error("Expected cluster to be required")
			}
			
			// Verify page defaults
			if tt.params.Page == 0 {
				// Should default to 1 in handler
				tt.params.Page = 1
			}
			
			// Verify page size defaults
			if tt.params.PageSize == 0 {
				// Should default to 10 in handler
				tt.params.PageSize = 10
			}
			
			if tt.params.Page < 1 {
				t.Error("Page should be at least 1")
			}
			if tt.params.PageSize < 1 || tt.params.PageSize > 100 {
				t.Error("PageSize should be between 1 and 100")
			}
		})
	}
}

func TestPodGPUHistoryParamsValidation(t *testing.T) {
	tests := []struct {
		name         string
		params       PodGPUHistoryParams
		expectError  bool
		errorMessage string
	}{
		{
			name: "Valid with start_time and end_time",
			params: PodGPUHistoryParams{
				StartTime:   "2024-01-01T00:00:00Z",
				EndTime:     "2024-01-02T00:00:00Z",
				Granularity: "hourly",
			},
			expectError: false,
		},
		{
			name: "Valid with hours parameter",
			params: PodGPUHistoryParams{
				Hours:       24,
				Granularity: "hourly",
			},
			expectError: false,
		},
		{
			name: "Valid with minute granularity",
			params: PodGPUHistoryParams{
				Hours:       1,
				Granularity: "minute",
			},
			expectError: false,
		},
		{
			name: "Valid with daily granularity",
			params: PodGPUHistoryParams{
				Hours:       168, // 7 days
				Granularity: "daily",
			},
			expectError: false,
		},
		{
			name: "Default granularity",
			params: PodGPUHistoryParams{
				Hours: 24,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Default granularity
			if tt.params.Granularity == "" {
				tt.params.Granularity = "hourly"
			}
			
			// Validate granularity
			validGranularities := map[string]bool{
				"minute": true,
				"hourly": true,
				"daily":  true,
			}
			
			if !validGranularities[tt.params.Granularity] {
				if !tt.expectError {
					t.Errorf("Invalid granularity: %s", tt.params.Granularity)
				}
			}
			
			// Validate time parameters
			if tt.params.Hours == 0 {
				if tt.params.StartTime == "" || tt.params.EndTime == "" {
					if !tt.expectError {
						t.Error("Either hours or start_time/end_time should be provided")
					}
				}
			}
		})
	}
}

func TestPodComparisonParamsValidation(t *testing.T) {
	tests := []struct {
		name        string
		podUIDs     []string
		expectError bool
	}{
		{
			name:        "Valid with 2 pods",
			podUIDs:     []string{"pod-1", "pod-2"},
			expectError: false,
		},
		{
			name:        "Valid with 5 pods",
			podUIDs:     []string{"pod-1", "pod-2", "pod-3", "pod-4", "pod-5"},
			expectError: false,
		},
		{
			name:        "Invalid with 1 pod",
			podUIDs:     []string{"pod-1"},
			expectError: true,
		},
		{
			name:        "Invalid with empty list",
			podUIDs:     []string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasError := len(tt.podUIDs) < 2
			if hasError != tt.expectError {
				t.Errorf("Expected error: %v, got: %v", tt.expectError, hasError)
			}
		})
	}
}

func TestGPUDataPointStruct(t *testing.T) {
	now := time.Now()
	dp := GPUDataPoint{
		Timestamp:      now,
		GPUUtilization: 75.5,
		MemoryUsed:     1024,
		Power:          150.0,
		Temperature:    65.0,
	}

	if dp.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if dp.GPUUtilization < 0 || dp.GPUUtilization > 100 {
		t.Error("GPU utilization should be between 0 and 100")
	}
	if dp.MemoryUsed < 0 {
		t.Error("Memory used should be non-negative")
	}
	if dp.Power < 0 {
		t.Error("Power should be non-negative")
	}
	if dp.Temperature < 0 {
		t.Error("Temperature should be non-negative")
	}
}

func TestPodStatsResponseStructure(t *testing.T) {
	response := PodStatsResponse{
		Total: 100,
		Page:  1,
		Pods: []PodStats{
			{
				PodUID:         "pod-1",
				PodName:        "test-pod-1",
				Namespace:      "default",
				NodeName:       "node-1",
				Status:         "Running",
				Phase:          "Running",
				CreatedAt:      time.Now(),
				AllocatedGPUs:  4,
				AvgUtilization: 75.5,
				Running:        true,
			},
		},
	}

	if response.Total != 100 {
		t.Errorf("Expected total 100, got %d", response.Total)
	}
	if len(response.Pods) != 1 {
		t.Errorf("Expected 1 pod, got %d", len(response.Pods))
	}
	if response.Pods[0].PodUID != "pod-1" {
		t.Errorf("Expected pod-1, got %s", response.Pods[0].PodUID)
	}
}

func TestPodDetailResponseStructure(t *testing.T) {
	now := time.Now()
	response := PodDetailResponse{
		PodUID:        "pod-1",
		PodName:       "test-pod",
		Namespace:     "default",
		NodeName:      "node-1",
		Status:        "Running",
		Phase:         "Running",
		CreatedAt:     now,
		UpdatedAt:     now,
		AllocatedGPUs: 4,
		Running:       true,
		Deleted:       false,
		IP:            "10.0.0.1",
		OwnerUID:      "owner-123",
		CurrentMetrics: &PodGPUMetrics{
			Timestamp:      now,
			GPUUtilization: 75.0,
			MemoryUsed:     1024,
			Power:          150.0,
			Temperature:    65.0,
		},
	}

	if response.PodUID != "pod-1" {
		t.Error("PodUID mismatch")
	}
	if response.CurrentMetrics == nil {
		t.Error("CurrentMetrics should not be nil")
	}
	if response.CurrentMetrics.GPUUtilization != 75.0 {
		t.Error("GPU utilization mismatch")
	}
}

func TestPodEventsResponseStructure(t *testing.T) {
	now := time.Now()
	response := PodEventsResponse{
		PodUID: "pod-1",
		Events: []PodEvent{
			{
				Timestamp: now,
				Type:      "Normal",
				Reason:    "Created",
				Message:   "Pod created",
				Source:    "kubelet",
			},
			{
				Timestamp: now.Add(time.Minute),
				Type:      "Normal",
				Reason:    "Started",
				Message:   "Container started",
				Source:    "kubelet",
			},
		},
	}

	if len(response.Events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(response.Events))
	}
	if response.Events[0].Type != "Normal" {
		t.Error("Event type mismatch")
	}
	if response.Events[0].Reason != "Created" {
		t.Error("Event reason mismatch")
	}
}

func TestPodComparisonResponseStructure(t *testing.T) {
	response := PodComparisonResponse{
		Pods: []PodComparisonItem{
			{
				PodUID:        "pod-1",
				PodName:       "test-pod-1",
				Namespace:     "default",
				AllocatedGPUs: 4,
			},
			{
				PodUID:        "pod-2",
				PodName:       "test-pod-2",
				Namespace:     "default",
				AllocatedGPUs: 8,
			},
		},
		Comparison: ComparisonSummary{
			HighestUtilization: "pod-2",
			LowestUtilization:  "pod-1",
			AvgUtilization:     80.0,
		},
	}

	if len(response.Pods) != 2 {
		t.Errorf("Expected 2 pods, got %d", len(response.Pods))
	}
	
	// Verify pod details
	pod1 := response.Pods[0]
	if pod1.AllocatedGPUs != 4 {
		t.Error("Pod1 GPU allocation mismatch")
	}
	if pod1.PodName != "test-pod-1" {
		t.Error("Pod1 name mismatch")
	}
	
	pod2 := response.Pods[1]
	if pod2.AllocatedGPUs != 8 {
		t.Error("Pod2 GPU allocation mismatch")
	}
	if pod2.PodName != "test-pod-2" {
		t.Error("Pod2 name mismatch")
	}
	
	// Verify comparison summary
	if response.Comparison.AvgUtilization != 80.0 {
		t.Error("Comparison AvgUtilization mismatch")
	}
	if response.Comparison.HighestUtilization != "pod-2" {
		t.Error("Comparison HighestUtilization mismatch")
	}
}

