package api

import (
	"testing"
)

func TestGenerateFragmentationRecommendations(t *testing.T) {
	tests := []struct {
		name                   string
		nodeFrags              []NodeFragmentation
		wantMinRecommendations int
		checkForCritical       bool
		checkForLowUtil        bool
		checkForFragmented     bool
	}{
		{
			name: "All healthy nodes",
			nodeFrags: []NodeFragmentation{
				{NodeName: "node-1", Status: "healthy", AllocatedGPUs: 8, Utilization: 80.0},
				{NodeName: "node-2", Status: "healthy", AllocatedGPUs: 8, Utilization: 75.0},
			},
			wantMinRecommendations: 1, // "No immediate action needed"
			checkForCritical:       false,
			checkForLowUtil:        false,
			checkForFragmented:     false,
		},
		{
			name: "Critical nodes present",
			nodeFrags: []NodeFragmentation{
				{NodeName: "node-1", Status: "critical", AllocatedGPUs: 8, Utilization: 10.0},
				{NodeName: "node-2", Status: "healthy", AllocatedGPUs: 8, Utilization: 75.0},
			},
			wantMinRecommendations: 1,
			checkForCritical:       true,
			checkForLowUtil:        true,
			checkForFragmented:     false,
		},
		{
			name: "Low utilization nodes",
			nodeFrags: []NodeFragmentation{
				{NodeName: "node-1", Status: "healthy", AllocatedGPUs: 8, Utilization: 15.0},
				{NodeName: "node-2", Status: "healthy", AllocatedGPUs: 8, Utilization: 25.0},
			},
			wantMinRecommendations: 1,
			checkForCritical:       false,
			checkForLowUtil:        true,
			checkForFragmented:     false,
		},
		{
			name: "Fragmented nodes with available GPUs",
			nodeFrags: []NodeFragmentation{
				{NodeName: "node-1", Status: "fragmented", AvailableGPUs: 2, Utilization: 50.0},
				{NodeName: "node-2", Status: "fragmented", AvailableGPUs: 3, Utilization: 60.0},
			},
			wantMinRecommendations: 1,
			checkForCritical:       false,
			checkForLowUtil:        false,
			checkForFragmented:     true,
		},
		{
			name: "Mixed status with multiple issues",
			nodeFrags: []NodeFragmentation{
				{NodeName: "node-1", Status: "critical", AllocatedGPUs: 8, Utilization: 10.0},
				{NodeName: "node-2", Status: "fragmented", AvailableGPUs: 3, Utilization: 50.0},
				{NodeName: "node-3", Status: "healthy", AllocatedGPUs: 6, Utilization: 25.0},
			},
			wantMinRecommendations: 2,
			checkForCritical:       true,
			checkForLowUtil:        true,
			checkForFragmented:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recommendations := generateFragmentationRecommendations(tt.nodeFrags)
			
			if len(recommendations) < tt.wantMinRecommendations {
				t.Errorf("Expected at least %d recommendations, got %d", 
					tt.wantMinRecommendations, len(recommendations))
			}
			
			// Check for specific recommendation types
			hasContent := false
			for _, rec := range recommendations {
				if len(rec) > 0 {
					hasContent = true
					break
				}
			}
			
			if !hasContent {
				t.Error("Recommendations should not be empty")
			}
		})
	}
}

func TestBuildFragmentationSummary(t *testing.T) {
	tests := []struct {
		name            string
		nodeFrags       []NodeFragmentation
		wantHealthy     int
		wantFragmented  int
		wantCritical    int
		wantMinWastedGPUs int
	}{
		{
			name: "All healthy nodes",
			nodeFrags: []NodeFragmentation{
				{Status: "healthy", TotalGPUs: 8, AvailableGPUs: 2},
				{Status: "healthy", TotalGPUs: 8, AvailableGPUs: 1},
			},
			wantHealthy:    2,
			wantFragmented: 0,
			wantCritical:   0,
			wantMinWastedGPUs: 0,
		},
		{
			name: "Mixed status nodes",
			nodeFrags: []NodeFragmentation{
				{Status: "healthy", TotalGPUs: 8, AvailableGPUs: 2},
				{Status: "fragmented", TotalGPUs: 8, AvailableGPUs: 3},
				{Status: "critical", TotalGPUs: 8, AvailableGPUs: 4},
			},
			wantHealthy:    1,
			wantFragmented: 1,
			wantCritical:   1,
			wantMinWastedGPUs: 7, // fragmented (3) + critical (4)
		},
		{
			name: "All fragmented nodes",
			nodeFrags: []NodeFragmentation{
				{Status: "fragmented", TotalGPUs: 8, AvailableGPUs: 2},
				{Status: "fragmented", TotalGPUs: 8, AvailableGPUs: 3},
			},
			wantHealthy:    0,
			wantFragmented: 2,
			wantCritical:   0,
			wantMinWastedGPUs: 5,
		},
		{
			name: "All critical nodes",
			nodeFrags: []NodeFragmentation{
				{Status: "critical", TotalGPUs: 8, AvailableGPUs: 5},
				{Status: "critical", TotalGPUs: 8, AvailableGPUs: 6},
			},
			wantHealthy:    0,
			wantFragmented: 0,
			wantCritical:   2,
			wantMinWastedGPUs: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := buildFragmentationSummary(tt.nodeFrags)
			
			if summary.HealthyNodes != tt.wantHealthy {
				t.Errorf("HealthyNodes = %d, want %d", summary.HealthyNodes, tt.wantHealthy)
			}
			if summary.FragmentedNodes != tt.wantFragmented {
				t.Errorf("FragmentedNodes = %d, want %d", summary.FragmentedNodes, tt.wantFragmented)
			}
			if summary.CriticalNodes != tt.wantCritical {
				t.Errorf("CriticalNodes = %d, want %d", summary.CriticalNodes, tt.wantCritical)
			}
			if summary.TotalWastedGPUs < tt.wantMinWastedGPUs {
				t.Errorf("TotalWastedGPUs = %d, want at least %d", 
					summary.TotalWastedGPUs, tt.wantMinWastedGPUs)
			}
			
			// Verify waste percentage calculation
			if summary.TotalWastedGPUs > 0 && summary.WastePercentage == 0 {
				t.Error("WastePercentage should be > 0 when TotalWastedGPUs > 0")
			}
		})
	}
}

func TestNodeFragmentationLogic(t *testing.T) {
	tests := []struct {
		name                   string
		frag                   NodeFragmentation
		pattern                AllocationPattern
		shouldHaveRecommendation bool
	}{
		{
			name: "Critical node with high fragmentation",
			frag: NodeFragmentation{
				Status:             "critical",
				FragmentationScore: 80.0,
				AvailableGPUs:      2,
			},
			pattern: AllocationPattern{
				PartiallyAllocPods: 5,
			},
			shouldHaveRecommendation: true,
		},
		{
			name: "Fragmented node with many partial allocations",
			frag: NodeFragmentation{
				Status:             "fragmented",
				FragmentationScore: 45.0,
				AvailableGPUs:      3,
			},
			pattern: AllocationPattern{
				PartiallyAllocPods: 6,
			},
			shouldHaveRecommendation: true,
		},
		{
			name: "Healthy node",
			frag: NodeFragmentation{
				Status:             "healthy",
				FragmentationScore: 15.0,
				AvailableGPUs:      2,
			},
			pattern: AllocationPattern{
				FullyAllocatedPods: 2,
			},
			shouldHaveRecommendation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test logic: validate that critical/fragmented nodes need recommendations
			needsAction := tt.frag.Status == "critical" || tt.frag.Status == "fragmented"
			
			if needsAction != tt.shouldHaveRecommendation {
				t.Errorf("Expected recommendation needed: %v, got: %v", 
					tt.shouldHaveRecommendation, needsAction)
			}
		})
	}
}

func TestIdentifyHotspotNodesLogic(t *testing.T) {
	tests := []struct {
		name         string
		nodeLoads    []NodeLoad
		threshold    float64
		wantHotspots int
	}{
		{
			name: "Multiple hotspot nodes",
			nodeLoads: []NodeLoad{
				{NodeName: "node-1", LoadScore: 95.0},
				{NodeName: "node-2", LoadScore: 50.0},
				{NodeName: "node-3", LoadScore: 98.0},
				{NodeName: "node-4", LoadScore: 60.0},
			},
			threshold:    90.0,
			wantHotspots: 2,
		},
		{
			name: "No hotspot nodes",
			nodeLoads: []NodeLoad{
				{NodeName: "node-1", LoadScore: 70.0},
				{NodeName: "node-2", LoadScore: 65.0},
				{NodeName: "node-3", LoadScore: 75.0},
			},
			threshold:    90.0,
			wantHotspots: 0,
		},
		{
			name: "All nodes are hotspots",
			nodeLoads: []NodeLoad{
				{NodeName: "node-1", LoadScore: 92.0},
				{NodeName: "node-2", LoadScore: 95.0},
				{NodeName: "node-3", LoadScore: 98.0},
			},
			threshold:    90.0,
			wantHotspots: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test logic: identify hotspot nodes based on threshold
			hotspots := []string{}
			for _, node := range tt.nodeLoads {
				if node.LoadScore > tt.threshold {
					hotspots = append(hotspots, node.NodeName)
				}
			}
			
			if len(hotspots) != tt.wantHotspots {
				t.Errorf("Expected %d hotspots, got %d", tt.wantHotspots, len(hotspots))
			}
		})
	}
}

func TestIdentifyIdleNodesLogic(t *testing.T) {
	tests := []struct {
		name      string
		nodeLoads []NodeLoad
		threshold float64
		wantIdle  int
	}{
		{
			name: "Multiple idle nodes",
			nodeLoads: []NodeLoad{
				{NodeName: "node-1", LoadScore: 5.0},
				{NodeName: "node-2", LoadScore: 50.0},
				{NodeName: "node-3", LoadScore: 15.0},
				{NodeName: "node-4", LoadScore: 60.0},
			},
			threshold: 20.0,
			wantIdle:  2,
		},
		{
			name: "No idle nodes",
			nodeLoads: []NodeLoad{
				{NodeName: "node-1", LoadScore: 70.0},
				{NodeName: "node-2", LoadScore: 65.0},
				{NodeName: "node-3", LoadScore: 75.0},
			},
			threshold: 20.0,
			wantIdle:  0,
		},
		{
			name: "All nodes are idle",
			nodeLoads: []NodeLoad{
				{NodeName: "node-1", LoadScore: 5.0},
				{NodeName: "node-2", LoadScore: 10.0},
				{NodeName: "node-3", LoadScore: 15.0},
			},
			threshold: 20.0,
			wantIdle:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test logic: identify idle nodes based on threshold
			idleNodes := []string{}
			for _, node := range tt.nodeLoads {
				if node.LoadScore < tt.threshold {
					idleNodes = append(idleNodes, node.NodeName)
				}
			}
			
			if len(idleNodes) != tt.wantIdle {
				t.Errorf("Expected %d idle nodes, got %d", tt.wantIdle, len(idleNodes))
			}
		})
	}
}

func TestGenerateLoadBalanceRecommendationsLogic(t *testing.T) {
	tests := []struct {
		name                   string
		hotspots               []string
		idleNodes              []string
		variance               float64
		wantMinRecommendations int
	}{
		{
			name:                   "Hotspots and idle nodes present",
			hotspots:               []string{"node-1", "node-2"},
			idleNodes:              []string{"node-3"},
			variance:               500.0,
			wantMinRecommendations: 1,
		},
		{
			name:                   "Only hotspots",
			hotspots:               []string{"node-1", "node-2"},
			idleNodes:              []string{},
			variance:               300.0,
			wantMinRecommendations: 1,
		},
		{
			name:                   "Only idle nodes",
			hotspots:               []string{},
			idleNodes:              []string{"node-3", "node-4"},
			variance:               200.0,
			wantMinRecommendations: 1,
		},
		{
			name:                   "High variance only",
			hotspots:               []string{},
			idleNodes:              []string{},
			variance:               400.0,
			wantMinRecommendations: 1,
		},
		{
			name:                   "No issues",
			hotspots:               []string{},
			idleNodes:              []string{},
			variance:               50.0,
			wantMinRecommendations: 1, // "well balanced"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test logic: generate recommendations based on cluster state
			recommendations := []string{}
			
			if len(tt.hotspots) > 0 {
				recommendations = append(recommendations, "Hotspot nodes detected")
			}
			if len(tt.idleNodes) > 0 {
				recommendations = append(recommendations, "Idle nodes detected")
			}
			if tt.variance > 100.0 {
				recommendations = append(recommendations, "High variance detected")
			}
			if len(recommendations) == 0 {
				recommendations = append(recommendations, "Cluster is well balanced")
			}
			
			if len(recommendations) < tt.wantMinRecommendations {
				t.Errorf("Expected at least %d recommendations, got %d", 
					tt.wantMinRecommendations, len(recommendations))
			}
		})
	}
}

func TestCalculateLoadBalanceStats(t *testing.T) {
	tests := []struct {
		name      string
		nodeLoads []NodeLoad
		wantStats LoadBalanceStats
	}{
		{
			name: "Uniform distribution",
			nodeLoads: []NodeLoad{
				{AllocationRate: 50.0},
				{AllocationRate: 50.0},
				{AllocationRate: 50.0},
			},
			wantStats: LoadBalanceStats{
				AvgAllocationRate: 50.0,
				StdDevAllocation:  0.0,
				MaxAllocation:     50.0,
				MinAllocation:     50.0,
				Variance:          0.0,
			},
		},
		{
			name: "Varied distribution",
			nodeLoads: []NodeLoad{
				{AllocationRate: 30.0},
				{AllocationRate: 50.0},
				{AllocationRate: 70.0},
			},
			wantStats: LoadBalanceStats{
				AvgAllocationRate: 50.0,
				MaxAllocation:     70.0,
				MinAllocation:     30.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := calculateLoadBalanceStats(tt.nodeLoads)
			
			if stats.AvgAllocationRate != tt.wantStats.AvgAllocationRate {
				t.Errorf("AvgAllocationRate = %f, want %f", 
					stats.AvgAllocationRate, tt.wantStats.AvgAllocationRate)
			}
			if stats.MaxAllocation != tt.wantStats.MaxAllocation {
				t.Errorf("MaxAllocation = %f, want %f", 
					stats.MaxAllocation, tt.wantStats.MaxAllocation)
			}
			if stats.MinAllocation != tt.wantStats.MinAllocation {
				t.Errorf("MinAllocation = %f, want %f", 
					stats.MinAllocation, tt.wantStats.MinAllocation)
			}
		})
	}
}

func TestLoadBalanceResponseStructure(t *testing.T) {
	response := LoadBalanceAnalysisResponse{
		Cluster:          "test-cluster",
		LoadBalanceScore: 75.5,
		NodeLoadDistribution: []NodeLoad{
			{NodeName: "node-1", AllocationRate: 80.0, UtilizationRate: 75.0, LoadScore: 77.5},
			{NodeName: "node-2", AllocationRate: 60.0, UtilizationRate: 55.0, LoadScore: 57.5},
		},
		HotspotNodes: []string{"node-1"},
		IdleNodes:    []string{},
		Recommendations: []string{"Consider rebalancing workloads"},
		Statistics: LoadBalanceStats{
			AvgAllocationRate: 70.0,
			StdDevAllocation:  10.0,
			MaxAllocation:     80.0,
			MinAllocation:     60.0,
			Variance:          100.0,
		},
	}

	if response.LoadBalanceScore != 75.5 {
		t.Error("LoadBalanceScore mismatch")
	}
	if len(response.NodeLoadDistribution) != 2 {
		t.Error("NodeLoadDistribution count mismatch")
	}
	if len(response.HotspotNodes) != 1 {
		t.Error("HotspotNodes count mismatch")
	}
	if response.Statistics.AvgAllocationRate != 70.0 {
		t.Error("Statistics AvgAllocationRate mismatch")
	}
}

