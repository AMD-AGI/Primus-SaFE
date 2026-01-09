// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"testing"
	"time"
)

func TestRealtimeStatusParamsValidation(t *testing.T) {
	tests := []struct {
		name    string
		params  RealtimeStatusParams
		isValid bool
	}{
		{
			name: "Valid with required cluster",
			params: RealtimeStatusParams{
				Cluster: "test-cluster",
			},
			isValid: true,
		},
		{
			name: "Valid with include fields",
			params: RealtimeStatusParams{
				Cluster: "test-cluster",
				Include: []string{"nodes", "alerts", "events"},
			},
			isValid: true,
		},
		{
			name: "Invalid without cluster",
			params: RealtimeStatusParams{
				Cluster: "",
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasError := tt.params.Cluster == ""
			if hasError == tt.isValid {
				t.Errorf("Expected valid=%v, got error=%v", tt.isValid, hasError)
			}
		})
	}
}

func TestRunningTasksParamsValidation(t *testing.T) {
	tests := []struct {
		name    string
		params  RunningTasksParams
		isValid bool
	}{
		{
			name: "Valid with required cluster",
			params: RunningTasksParams{
				Cluster: "test-cluster",
			},
			isValid: true,
		},
		{
			name: "Valid with namespace filter",
			params: RunningTasksParams{
				Cluster:   "test-cluster",
				Namespace: "default",
			},
			isValid: true,
		},
		{
			name: "Invalid without cluster",
			params: RunningTasksParams{
				Cluster: "",
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasError := tt.params.Cluster == ""
			if hasError == tt.isValid {
				t.Errorf("Expected valid=%v, got error=%v", tt.isValid, hasError)
			}
		})
	}
}

func TestGPUUsageSummaryCalculations(t *testing.T) {
	tests := []struct {
		name            string
		summary         GPUUsageSummary
		expectValid     bool
		expectRatesValid bool
	}{
		{
			name: "Valid summary with 50% allocation",
			summary: GPUUsageSummary{
				TotalGPUs:       100,
				AllocatedGPUs:   50,
				UtilizedGPUs:    40,
				AllocationRate:  50.0,
				UtilizationRate: 40.0,
			},
			expectValid:      true,
			expectRatesValid: true,
		},
		{
			name: "Valid summary with 100% allocation",
			summary: GPUUsageSummary{
				TotalGPUs:       100,
				AllocatedGPUs:   100,
				UtilizedGPUs:    100,
				AllocationRate:  100.0,
				UtilizationRate: 100.0,
			},
			expectValid:      true,
			expectRatesValid: true,
		},
		{
			name: "Valid summary with 0% allocation",
			summary: GPUUsageSummary{
				TotalGPUs:       100,
				AllocatedGPUs:   0,
				UtilizedGPUs:    0,
				AllocationRate:  0.0,
				UtilizationRate: 0.0,
			},
			expectValid:      true,
			expectRatesValid: true,
		},
		{
			name: "Valid summary with partial utilization",
			summary: GPUUsageSummary{
				TotalGPUs:       96,
				AllocatedGPUs:   84,
				UtilizedGPUs:    56,
				AllocationRate:  87.5,
				UtilizationRate: 58.33,
			},
			expectValid:      true,
			expectRatesValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate basic constraints
			if tt.summary.AllocatedGPUs > tt.summary.TotalGPUs {
				if tt.expectValid {
					t.Error("AllocatedGPUs should not exceed TotalGPUs")
				}
			}
			if tt.summary.UtilizedGPUs > tt.summary.AllocatedGPUs {
				if tt.expectValid {
					t.Error("UtilizedGPUs should not exceed AllocatedGPUs")
				}
			}
			
			// Validate rates
			if tt.expectRatesValid {
				if tt.summary.AllocationRate < 0 || tt.summary.AllocationRate > 100 {
					t.Error("AllocationRate should be between 0 and 100")
				}
				if tt.summary.UtilizationRate < 0 || tt.summary.UtilizationRate > 100 {
					t.Error("UtilizationRate should be between 0 and 100")
				}
			}
		})
	}
}

func TestResourceAvailabilityStructure(t *testing.T) {
	tests := []struct {
		name         string
		availability ResourceAvailability
		expectValid  bool
	}{
		{
			name: "Valid availability",
			availability: ResourceAvailability{
				AvailableGPUs:    12,
				AvailableNodes:   2,
				MaxContiguousGPU: 8,
			},
			expectValid: true,
		},
		{
			name: "Zero availability",
			availability: ResourceAvailability{
				AvailableGPUs:    0,
				AvailableNodes:   0,
				MaxContiguousGPU: 0,
			},
			expectValid: true,
		},
		{
			name: "High availability",
			availability: ResourceAvailability{
				AvailableGPUs:    48,
				AvailableNodes:   6,
				MaxContiguousGPU: 8,
			},
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate constraints
			if tt.availability.MaxContiguousGPU > tt.availability.AvailableGPUs {
				if tt.expectValid {
					t.Error("MaxContiguousGPU should not exceed AvailableGPUs")
				}
			}
			if tt.availability.AvailableGPUs < 0 {
				t.Error("AvailableGPUs should be non-negative")
			}
			if tt.availability.AvailableNodes < 0 {
				t.Error("AvailableNodes should be non-negative")
			}
		})
	}
}

func TestAlertStructure(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		alert  Alert
		isValid bool
	}{
		{
			name: "Valid critical alert",
			alert: Alert{
				ID:        "alert-1",
				Severity:  "critical",
				Type:      "GPU",
				Message:   "High GPU fragmentation detected",
				Timestamp: now,
				Source:    "fragmentation-analyzer",
			},
			isValid: true,
		},
		{
			name: "Valid warning alert",
			alert: Alert{
				ID:        "alert-2",
				Severity:  "warning",
				Type:      "Load",
				Message:   "Unbalanced workload distribution",
				Timestamp: now,
				Source:    "load-balancer",
			},
			isValid: true,
		},
		{
			name: "Valid info alert",
			alert: Alert{
				ID:        "alert-3",
				Severity:  "info",
				Type:      "System",
				Message:   "New node added to cluster",
				Timestamp: now,
				Source:    "cluster-manager",
			},
			isValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.alert.ID == "" {
				t.Error("Alert ID should not be empty")
			}
			if tt.alert.Severity == "" {
				t.Error("Alert Severity should not be empty")
			}
			if tt.alert.Message == "" {
				t.Error("Alert Message should not be empty")
			}
			if tt.alert.Timestamp.IsZero() {
				t.Error("Alert Timestamp should not be zero")
			}
			
			// Validate severity
			validSeverities := map[string]bool{
				"critical": true,
				"warning":  true,
				"info":     true,
			}
			if !validSeverities[tt.alert.Severity] {
				t.Errorf("Invalid severity: %s", tt.alert.Severity)
			}
		})
	}
}

func TestNodeStatusStructure(t *testing.T) {
	tests := []struct {
		name       string
		nodeStatus NodeStatus
		isValid    bool
	}{
		{
			name: "Valid ready node",
			nodeStatus: NodeStatus{
				NodeName:      "node-1",
				Status:        "Ready",
				TotalGPUs:     8,
				AllocatedGPUs: 6,
				Utilization:   75.0,
			},
			isValid: true,
		},
		{
			name: "Valid idle node",
			nodeStatus: NodeStatus{
				NodeName:      "node-2",
				Status:        "Ready",
				TotalGPUs:     8,
				AllocatedGPUs: 0,
				Utilization:   0.0,
			},
			isValid: true,
		},
		{
			name: "Valid fully allocated node",
			nodeStatus: NodeStatus{
				NodeName:      "node-3",
				Status:        "Ready",
				TotalGPUs:     8,
				AllocatedGPUs: 8,
				Utilization:   100.0,
			},
			isValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.nodeStatus.NodeName == "" {
				t.Error("NodeName should not be empty")
			}
			if tt.nodeStatus.AllocatedGPUs > tt.nodeStatus.TotalGPUs {
				if tt.isValid {
					t.Error("AllocatedGPUs should not exceed TotalGPUs")
				}
			}
			if tt.nodeStatus.Utilization < 0 || tt.nodeStatus.Utilization > 100 {
				t.Error("Utilization should be between 0 and 100")
			}
		})
	}
}

func TestEventStructure(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		event Event
	}{
		{
			name: "Pod created event",
			event: Event{
				Timestamp: now,
				Type:      "PodCreated",
				Object:    "default/test-pod",
				Message:   "Pod created with 4 GPUs",
			},
		},
		{
			name: "Pod deleted event",
			event: Event{
				Timestamp: now,
				Type:      "PodDeleted",
				Object:    "default/test-pod",
				Message:   "Pod deleted",
			},
		},
		{
			name: "Node added event",
			event: Event{
				Timestamp: now,
				Type:      "NodeAdded",
				Object:    "node-4",
				Message:   "New node with 8 GPUs added",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.event.Timestamp.IsZero() {
				t.Error("Event Timestamp should not be zero")
			}
			if tt.event.Type == "" {
				t.Error("Event Type should not be empty")
			}
			if tt.event.Object == "" {
				t.Error("Event Object should not be empty")
			}
			if tt.event.Message == "" {
				t.Error("Event Message should not be empty")
			}
		})
	}
}

func TestRunningTaskStructure(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		task RunningTask
	}{
		{
			name: "Valid running task",
			task: RunningTask{
				PodUID:        "pod-uid-123",
				PodName:       "test-pod",
				Namespace:     "default",
				WorkloadType:  "Job",
				WorkloadName:  "training-job",
				NodeName:      "node-1",
				AllocatedGPUs: 4,
				RunningTime:   3600,
				StartedAt:     now.Add(-time.Hour),
				Owner:         "user@example.com",
			},
		},
		{
			name: "Long running task",
			task: RunningTask{
				PodUID:        "pod-uid-456",
				PodName:       "long-pod",
				Namespace:     "ml",
				WorkloadType:  "Deployment",
				WorkloadName:  "inference-service",
				NodeName:      "node-2",
				AllocatedGPUs: 8,
				RunningTime:   86400, // 24 hours
				StartedAt:     now.Add(-24 * time.Hour),
				Owner:         "admin@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.task.PodUID == "" {
				t.Error("PodUID should not be empty")
			}
			if tt.task.PodName == "" {
				t.Error("PodName should not be empty")
			}
			if tt.task.Namespace == "" {
				t.Error("Namespace should not be empty")
			}
			if tt.task.AllocatedGPUs <= 0 {
				t.Error("AllocatedGPUs should be positive")
			}
			if tt.task.RunningTime < 0 {
				t.Error("RunningTime should be non-negative")
			}
			if tt.task.StartedAt.IsZero() {
				t.Error("StartedAt should not be zero")
			}
		})
	}
}

func TestRealtimeStatusResponseStructure(t *testing.T) {
	now := time.Now()
	response := RealtimeStatusResponse{
		Cluster:   "test-cluster",
		Timestamp: now,
		CurrentGPUUsage: GPUUsageSummary{
			TotalGPUs:       96,
			AllocatedGPUs:   84,
			UtilizedGPUs:    56,
			AllocationRate:  87.5,
			UtilizationRate: 75.5,
		},
		RunningTasks: 14,
		AvailableResources: ResourceAvailability{
			AvailableGPUs:    12,
			AvailableNodes:   2,
			MaxContiguousGPU: 8,
		},
		Alerts: []Alert{
			{
				ID:        "alert-1",
				Severity:  "warning",
				Type:      "GPU",
				Message:   "High GPU fragmentation",
				Timestamp: now,
				Source:    "analyzer",
			},
		},
		Nodes: []NodeStatus{
			{
				NodeName:      "node-1",
				Status:        "Ready",
				TotalGPUs:     8,
				AllocatedGPUs: 7,
				Utilization:   85.0,
			},
		},
		RecentEvents: []Event{
			{
				Timestamp: now,
				Type:      "PodCreated",
				Object:    "default/test-pod",
				Message:   "Pod created",
			},
		},
	}

	if response.Cluster != "test-cluster" {
		t.Error("Cluster mismatch")
	}
	if response.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if response.CurrentGPUUsage.TotalGPUs != 96 {
		t.Error("TotalGPUs mismatch")
	}
	if response.RunningTasks != 14 {
		t.Error("RunningTasks mismatch")
	}
	if len(response.Alerts) != 1 {
		t.Error("Alerts count mismatch")
	}
	if len(response.Nodes) != 1 {
		t.Error("Nodes count mismatch")
	}
	if len(response.RecentEvents) != 1 {
		t.Error("RecentEvents count mismatch")
	}
}

func TestRunningTasksResponseStructure(t *testing.T) {
	now := time.Now()
	response := RunningTasksResponse{
		Cluster:    "test-cluster",
		Timestamp:  now,
		TotalTasks: 2,
		Tasks: []RunningTask{
			{
				PodUID:        "pod-1",
				PodName:       "task-1",
				Namespace:     "default",
				WorkloadType:  "Job",
				WorkloadName:  "job-1",
				NodeName:      "node-1",
				AllocatedGPUs: 4,
				RunningTime:   3600,
				StartedAt:     now.Add(-time.Hour),
			},
			{
				PodUID:        "pod-2",
				PodName:       "task-2",
				Namespace:     "ml",
				WorkloadType:  "Deployment",
				WorkloadName:  "deploy-1",
				NodeName:      "node-2",
				AllocatedGPUs: 8,
				RunningTime:   7200,
				StartedAt:     now.Add(-2 * time.Hour),
			},
		},
	}

	if response.Cluster != "test-cluster" {
		t.Error("Cluster mismatch")
	}
	if response.TotalTasks != 2 {
		t.Error("TotalTasks mismatch")
	}
	if len(response.Tasks) != 2 {
		t.Error("Tasks count mismatch")
	}
	if response.Tasks[0].AllocatedGPUs != 4 {
		t.Error("First task GPU allocation mismatch")
	}
	if response.Tasks[1].AllocatedGPUs != 8 {
		t.Error("Second task GPU allocation mismatch")
	}
}

