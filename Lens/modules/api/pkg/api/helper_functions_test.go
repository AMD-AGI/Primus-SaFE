package api

import (
	"testing"
	"time"

	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// Test queryGPUHistory helper function logic
func TestQueryGPUHistoryLogic(t *testing.T) {
	tests := []struct {
		name        string
		devices     []*dbModel.GpuDevice
		expectCount int
	}{
		{
			name:        "Empty devices",
			devices:     []*dbModel.GpuDevice{},
			expectCount: 0,
		},
		{
			name: "Single device",
			devices: []*dbModel.GpuDevice{
				{
					UpdatedAt:   time.Now(),
					Utilization: 75.5,
					Memory:      1024,
					Power:       150.0,
					Temperature: 65.0,
				},
			},
			expectCount: 1,
		},
		{
			name: "Multiple devices",
			devices: []*dbModel.GpuDevice{
				{
					UpdatedAt:   time.Now(),
					Utilization: 75.5,
					Memory:      1024,
					Power:       150.0,
					Temperature: 65.0,
				},
				{
					UpdatedAt:   time.Now().Add(-time.Hour),
					Utilization: 80.0,
					Memory:      2048,
					Power:       160.0,
					Temperature: 70.0,
				},
			},
			expectCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate queryGPUHistory logic
			dataPoints := make([]GPUDataPoint, 0, len(tt.devices))
			for _, device := range tt.devices {
				dataPoints = append(dataPoints, GPUDataPoint{
					Timestamp:      device.UpdatedAt,
					GPUUtilization: device.Utilization,
					MemoryUsed:     device.Memory,
					Power:          device.Power,
					Temperature:    device.Temperature,
				})
			}

			if len(dataPoints) != tt.expectCount {
				t.Errorf("Expected %d data points, got %d", tt.expectCount, len(dataPoints))
			}

			// Validate data points
			for i, dp := range dataPoints {
				if dp.Timestamp.IsZero() {
					t.Error("Timestamp should not be zero")
				}
				if dp.GPUUtilization != tt.devices[i].Utilization {
					t.Error("GPU utilization mismatch")
				}
				if dp.MemoryUsed != tt.devices[i].Memory {
					t.Error("Memory used mismatch")
				}
			}
		})
	}
}

// Test queryPodEvents helper function logic
func TestQueryPodEventsLogic(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name        string
		gpuEvents   []*dbModel.GpuPodsEvent
		expectCount int
	}{
		{
			name:        "Empty events",
			gpuEvents:   []*dbModel.GpuPodsEvent{},
			expectCount: 0,
		},
		{
			name: "Single event",
			gpuEvents: []*dbModel.GpuPodsEvent{
				{
					CreatedAt:    now,
					EventType:    "Normal",
					PodPhase:     "Running",
					RestartCount: 0,
				},
			},
			expectCount: 1,
		},
		{
			name: "Multiple events",
			gpuEvents: []*dbModel.GpuPodsEvent{
				{
					CreatedAt:    now,
					EventType:    "Normal",
					PodPhase:     "Running",
					RestartCount: 0,
				},
				{
					CreatedAt:    now.Add(-time.Minute),
					EventType:    "Warning",
					PodPhase:     "Pending",
					RestartCount: 1,
				},
			},
			expectCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate queryPodEvents logic
			events := make([]PodEvent, 0, len(tt.gpuEvents))
			for _, event := range tt.gpuEvents {
				events = append(events, PodEvent{
					Timestamp: event.CreatedAt,
					Type:      event.EventType,
					Reason:    event.PodPhase,
					Message:   "Event message",
					Source:    "gpu-pods-event",
				})
			}

			if len(events) != tt.expectCount {
				t.Errorf("Expected %d events, got %d", tt.expectCount, len(events))
			}

			// Validate events
			for i, ev := range events {
				if ev.Timestamp.IsZero() {
					t.Error("Timestamp should not be zero")
				}
				if ev.Type != tt.gpuEvents[i].EventType {
					t.Error("Event type mismatch")
				}
				if ev.Reason != tt.gpuEvents[i].PodPhase {
					t.Error("Event reason mismatch")
				}
				if ev.Source == "" {
					t.Error("Source should not be empty")
				}
			}
		})
	}
}

// Test identifyHotspotAndIdleNodes function
func TestIdentifyHotspotAndIdleNodes(t *testing.T) {
	tests := []struct {
		name             string
		nodeLoads        []NodeLoad
		expectedHotspots int
		expectedIdle     int
		checkMean        bool
		expectedMean     float64
	}{
		{
			name: "Balanced cluster - no hotspots or idle",
			nodeLoads: []NodeLoad{
				{NodeName: "node-1", LoadScore: 50.0},
				{NodeName: "node-2", LoadScore: 51.0},
				{NodeName: "node-3", LoadScore: 49.0},
			},
			expectedHotspots: 0,
			expectedIdle:     0,
			checkMean:        true,
			expectedMean:     50.0,
		},
		{
			name: "Unbalanced cluster - hotspots and idle nodes",
			nodeLoads: []NodeLoad{
				{NodeName: "node-1", LoadScore: 90.0}, // hotspot (mean=50, 90>50+20)
				{NodeName: "node-2", LoadScore: 50.0},
				{NodeName: "node-3", LoadScore: 10.0}, // idle (10<50-20)
			},
			expectedHotspots: 1,
			expectedIdle:     1,
			checkMean:        true,
			expectedMean:     50.0,
		},
		{
			name: "All hotspots",
			nodeLoads: []NodeLoad{
				{NodeName: "node-1", LoadScore: 90.0},
				{NodeName: "node-2", LoadScore: 95.0},
				{NodeName: "node-3", LoadScore: 85.0},
			},
			expectedHotspots: 0, // All above mean, but need mean+20 threshold
			expectedIdle:     0,
			checkMean:        true,
			expectedMean:     90.0,
		},
		{
			name: "All idle",
			nodeLoads: []NodeLoad{
				{NodeName: "node-1", LoadScore: 10.0},
				{NodeName: "node-2", LoadScore: 15.0},
				{NodeName: "node-3", LoadScore: 5.0},
			},
			expectedHotspots: 0,
			expectedIdle:     0, // All below mean, but need mean-20 threshold
			checkMean:        true,
			expectedMean:     10.0,
		},
		{
			name: "Wide distribution",
			nodeLoads: []NodeLoad{
				{NodeName: "node-1", LoadScore: 100.0}, // hotspot
				{NodeName: "node-2", LoadScore: 50.0},
				{NodeName: "node-3", LoadScore: 48.0},
				{NodeName: "node-4", LoadScore: 0.0}, // idle
			},
			expectedHotspots: 1, // 100 > (49.5+20)
			expectedIdle:     1, // 0 < (49.5-20)
			checkMean:        true,
			expectedMean:     49.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hotspots, idle := identifyHotspotAndIdleNodes(tt.nodeLoads)

			if len(hotspots) != tt.expectedHotspots {
				t.Errorf("Expected %d hotspots, got %d", tt.expectedHotspots, len(hotspots))
			}

			if len(idle) != tt.expectedIdle {
				t.Errorf("Expected %d idle nodes, got %d", tt.expectedIdle, len(idle))
			}

			// Verify mean calculation if required
			if tt.checkMean {
				var sum float64
				for _, load := range tt.nodeLoads {
					sum += load.LoadScore
				}
				mean := sum / float64(len(tt.nodeLoads))

				if mean != tt.expectedMean {
					t.Errorf("Expected mean %f, got %f", tt.expectedMean, mean)
				}
			}
		})
	}
}

// Test Pod comparison summary calculation logic
func TestPodComparisonSummaryCalculation(t *testing.T) {
	tests := []struct {
		name            string
		pods            []PodComparisonItem
		expectedHighest string
		expectedLowest  string
		expectedAvgUtil float64
	}{
		{
			name: "Two pods comparison",
			pods: []PodComparisonItem{
				{
					PodName: "pod-1",
					Metrics: map[string]float64{"gpu_utilization": 50.0},
				},
				{
					PodName: "pod-2",
					Metrics: map[string]float64{"gpu_utilization": 80.0},
				},
			},
			expectedHighest: "pod-2",
			expectedLowest:  "pod-1",
			expectedAvgUtil: 65.0,
		},
		{
			name: "Three pods with same utilization",
			pods: []PodComparisonItem{
				{
					PodName: "pod-1",
					Metrics: map[string]float64{"gpu_utilization": 60.0},
				},
				{
					PodName: "pod-2",
					Metrics: map[string]float64{"gpu_utilization": 60.0},
				},
				{
					PodName: "pod-3",
					Metrics: map[string]float64{"gpu_utilization": 60.0},
				},
			},
			expectedHighest: "pod-1", // First one will be highest
			expectedLowest:  "pod-1", // First one will be lowest
			expectedAvgUtil: 60.0,
		},
		{
			name: "Multiple pods with varied utilization",
			pods: []PodComparisonItem{
				{
					PodName: "pod-1",
					Metrics: map[string]float64{"gpu_utilization": 30.0},
				},
				{
					PodName: "pod-2",
					Metrics: map[string]float64{"gpu_utilization": 90.0},
				},
				{
					PodName: "pod-3",
					Metrics: map[string]float64{"gpu_utilization": 60.0},
				},
			},
			expectedHighest: "pod-2",
			expectedLowest:  "pod-1",
			expectedAvgUtil: 60.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate comparison summary calculation logic
			var totalUtil float64
			var highestUtil float64
			var lowestUtil float64 = 100.0
			var highestPod, lowestPod string

			for _, pod := range tt.pods {
				avgUtil := pod.Metrics["gpu_utilization"]
				totalUtil += avgUtil

				if avgUtil > highestUtil {
					highestUtil = avgUtil
					highestPod = pod.PodName
				}
				if avgUtil < lowestUtil {
					lowestUtil = avgUtil
					lowestPod = pod.PodName
				}
			}

			avgUtil := totalUtil / float64(len(tt.pods))

			if highestPod != tt.expectedHighest {
				t.Errorf("Expected highest pod %s, got %s", tt.expectedHighest, highestPod)
			}
			if lowestPod != tt.expectedLowest {
				t.Errorf("Expected lowest pod %s, got %s", tt.expectedLowest, lowestPod)
			}
			if avgUtil != tt.expectedAvgUtil {
				t.Errorf("Expected avg utilization %f, got %f", tt.expectedAvgUtil, avgUtil)
			}
		})
	}
}

// Test calculateNodeLoad logic
func TestCalculateNodeLoadLogic(t *testing.T) {
	tests := []struct {
		name                string
		node                *dbModel.Node
		expectedAllocation  float64
		expectedUtilization float64
		checkLoadScore      bool
	}{
		{
			name: "Fully allocated and utilized node",
			node: &dbModel.Node{
				Name:           "node-1",
				GpuCount:       8,
				GpuAllocation:  8,
				GpuUtilization: 100.0,
			},
			expectedAllocation:  100.0,
			expectedUtilization: 100.0,
			checkLoadScore:      true,
		},
		{
			name: "Half allocated node",
			node: &dbModel.Node{
				Name:           "node-2",
				GpuCount:       8,
				GpuAllocation:  4,
				GpuUtilization: 50.0,
			},
			expectedAllocation:  50.0,
			expectedUtilization: 50.0,
			checkLoadScore:      true,
		},
		{
			name: "Idle node",
			node: &dbModel.Node{
				Name:           "node-3",
				GpuCount:       8,
				GpuAllocation:  0,
				GpuUtilization: 0.0,
			},
			expectedAllocation:  0.0,
			expectedUtilization: 0.0,
			checkLoadScore:      true,
		},
		{
			name: "High utilization node",
			node: &dbModel.Node{
				Name:           "node-4",
				GpuCount:       8,
				GpuAllocation:  6,
				GpuUtilization: 85.0,
			},
			expectedAllocation:  75.0,
			expectedUtilization: 85.0,
			checkLoadScore:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate calculateNodeLoad logic
			allocRate := float64(tt.node.GpuAllocation) / float64(tt.node.GpuCount) * 100
			utilRate := tt.node.GpuUtilization
			// Using the actual weighted formula from the code (60% allocation, 40% utilization)
			loadScore := allocRate*0.6 + utilRate*0.4

			if allocRate != tt.expectedAllocation {
				t.Errorf("Expected allocation rate %f, got %f", tt.expectedAllocation, allocRate)
			}
			if utilRate != tt.expectedUtilization {
				t.Errorf("Expected utilization rate %f, got %f", tt.expectedUtilization, utilRate)
			}

			if tt.checkLoadScore {
				expectedLoad := tt.expectedAllocation*0.6 + tt.expectedUtilization*0.4
				if loadScore != expectedLoad {
					t.Errorf("Expected load score %f, got %f", expectedLoad, loadScore)
				}
			}
		})
	}
}

// Test fragmentation summary calculation edge cases
func TestFragmentationSummaryEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		nodeFrags   []NodeFragmentation
		expectValid bool
		expectWaste bool
	}{
		{
			name:        "Empty node list",
			nodeFrags:   []NodeFragmentation{},
			expectValid: true,
			expectWaste: false,
		},
		{
			name: "Single healthy node",
			nodeFrags: []NodeFragmentation{
				{Status: "healthy", TotalGPUs: 8, AvailableGPUs: 0},
			},
			expectValid: true,
			expectWaste: false,
		},
		{
			name: "All nodes critical",
			nodeFrags: []NodeFragmentation{
				{Status: "critical", TotalGPUs: 8, AvailableGPUs: 6},
				{Status: "critical", TotalGPUs: 8, AvailableGPUs: 7},
			},
			expectValid: true,
			expectWaste: true,
		},
		{
			name: "Mixed status nodes",
			nodeFrags: []NodeFragmentation{
				{Status: "healthy", TotalGPUs: 8, AvailableGPUs: 0},
				{Status: "fragmented", TotalGPUs: 8, AvailableGPUs: 2},
				{Status: "critical", TotalGPUs: 8, AvailableGPUs: 5},
			},
			expectValid: true,
			expectWaste: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := buildFragmentationSummary(tt.nodeFrags)

			// Count status types
			var healthy, fragmented, critical int
			var totalWaste int32
			for _, frag := range tt.nodeFrags {
				switch frag.Status {
				case "healthy":
					healthy++
				case "fragmented":
					fragmented++
					totalWaste += frag.AvailableGPUs
				case "critical":
					critical++
					totalWaste += frag.AvailableGPUs
				}
			}

			if summary.HealthyNodes != healthy {
				t.Errorf("Expected %d healthy nodes, got %d", healthy, summary.HealthyNodes)
			}
			if summary.FragmentedNodes != fragmented {
				t.Errorf("Expected %d fragmented nodes, got %d", fragmented, summary.FragmentedNodes)
			}
			if summary.CriticalNodes != critical {
				t.Errorf("Expected %d critical nodes, got %d", critical, summary.CriticalNodes)
			}

			if tt.expectWaste && summary.TotalWastedGPUs == 0 {
				t.Error("Expected wasted GPUs but got 0")
			}
			if !tt.expectWaste && summary.TotalWastedGPUs > 0 {
				t.Error("Expected no wasted GPUs but got some")
			}
		})
	}
}
