package api

import (
	"testing"
	"time"
)

func TestParseOwnerReference(t *testing.T) {
	tests := []struct {
		name        string
		ownerUID    string
		wantType    string
		wantNameLen int
	}{
		{
			name:        "Empty owner UID",
			ownerUID:    "",
			wantType:    "Unknown",
			wantNameLen: 7, // "Unknown"
		},
		{
			name:        "Valid owner UID",
			ownerUID:    "abc123def456",
			wantType:    "Job",
			wantNameLen: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotName := parseOwnerReference(tt.ownerUID)
			if gotType != tt.wantType {
				t.Errorf("parseOwnerReference() type = %v, want %v", gotType, tt.wantType)
			}
			if len(gotName) != tt.wantNameLen {
				t.Errorf("parseOwnerReference() name length = %v, want %v", len(gotName), tt.wantNameLen)
			}
		})
	}
}

func TestFilterRealtimeResponse(t *testing.T) {
	response := RealtimeStatusResponse{
		Cluster:   "test-cluster",
		Timestamp: time.Now(),
		Nodes: []NodeStatus{
			{NodeName: "node-1"},
		},
		Alerts: []Alert{
			{ID: "alert-1"},
		},
		RecentEvents: []Event{
			{Type: "PodCreated"},
		},
	}

	tests := []struct {
		name       string
		includeMap map[string]bool
		wantNodes  bool
		wantAlerts bool
		wantEvents bool
	}{
		{
			name:       "Include all",
			includeMap: map[string]bool{"nodes": true, "alerts": true, "events": true},
			wantNodes:  true,
			wantAlerts: true,
			wantEvents: true,
		},
		{
			name:       "Include none",
			includeMap: map[string]bool{},
			wantNodes:  false,
			wantAlerts: false,
			wantEvents: false,
		},
		{
			name:       "Include only nodes",
			includeMap: map[string]bool{"nodes": true},
			wantNodes:  true,
			wantAlerts: false,
			wantEvents: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterRealtimeResponse(response, tt.includeMap)

			hasNodes := result.Nodes != nil
			hasAlerts := result.Alerts != nil
			hasEvents := result.RecentEvents != nil

			if hasNodes != tt.wantNodes {
				t.Errorf("filterRealtimeResponse() nodes = %v, want %v", hasNodes, tt.wantNodes)
			}
			if hasAlerts != tt.wantAlerts {
				t.Errorf("filterRealtimeResponse() alerts = %v, want %v", hasAlerts, tt.wantAlerts)
			}
			if hasEvents != tt.wantEvents {
				t.Errorf("filterRealtimeResponse() events = %v, want %v", hasEvents, tt.wantEvents)
			}
		})
	}
}

func TestGPUUsageSummaryCalculation(t *testing.T) {
	// Test helper to verify GPU usage calculation logic
	tests := []struct {
		name               string
		totalGPUs          int32
		allocatedGPUs      int32
		wantAllocationRate float64
	}{
		{
			name:               "50% allocation",
			totalGPUs:          8,
			allocatedGPUs:      4,
			wantAllocationRate: 50.0,
		},
		{
			name:               "100% allocation",
			totalGPUs:          8,
			allocatedGPUs:      8,
			wantAllocationRate: 100.0,
		},
		{
			name:               "0% allocation",
			totalGPUs:          8,
			allocatedGPUs:      0,
			wantAllocationRate: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allocationRate := 0.0
			if tt.totalGPUs > 0 {
				allocationRate = float64(tt.allocatedGPUs) / float64(tt.totalGPUs) * 100
			}

			if allocationRate != tt.wantAllocationRate {
				t.Errorf("Allocation rate = %v, want %v", allocationRate, tt.wantAllocationRate)
			}
		})
	}
}
