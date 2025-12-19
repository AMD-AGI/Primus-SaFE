package api

import (
	"testing"

	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

func TestDetermineFragmentationStatus(t *testing.T) {
	tests := []struct {
		name     string
		score    float64
		expected string
	}{
		{
			name:     "Healthy node",
			score:    20.0,
			expected: "healthy",
		},
		{
			name:     "Fragmented node",
			score:    45.0,
			expected: "fragmented",
		},
		{
			name:     "Critical node",
			score:    75.0,
			expected: "critical",
		},
		{
			name:     "Edge case - healthy/fragmented boundary",
			score:    30.0,
			expected: "fragmented",
		},
		{
			name:     "Edge case - fragmented/critical boundary",
			score:    60.0,
			expected: "critical",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineFragmentationStatus(tt.score)
			if result != tt.expected {
				t.Errorf("determineFragmentationStatus(%f) = %s; want %s",
					tt.score, result, tt.expected)
			}
		})
	}
}

func TestCalculatePartialAllocationPenalty(t *testing.T) {
	tests := []struct {
		name      string
		pods      []*dbModel.GpuPods
		totalGPUs int32
		wantRange [2]float64 // min and max expected values
	}{
		{
			name:      "No pods",
			pods:      []*dbModel.GpuPods{},
			totalGPUs: 8,
			wantRange: [2]float64{0.0, 0.0},
		},
		{
			name: "Many small allocations on large node",
			pods: []*dbModel.GpuPods{
				{GpuAllocated: 1},
				{GpuAllocated: 1},
				{GpuAllocated: 1},
			},
			totalGPUs: 8,
			wantRange: [2]float64{0.3, 0.4},
		},
		{
			name: "Large allocations",
			pods: []*dbModel.GpuPods{
				{GpuAllocated: 4},
				{GpuAllocated: 4},
			},
			totalGPUs: 8,
			wantRange: [2]float64{0.0, 0.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePartialAllocationPenalty(tt.pods, tt.totalGPUs)
			if result < tt.wantRange[0] || result > tt.wantRange[1] {
				t.Errorf("calculatePartialAllocationPenalty() = %f; want between %f and %f",
					result, tt.wantRange[0], tt.wantRange[1])
			}
		})
	}
}

func TestBuildAllocationPattern(t *testing.T) {
	tests := []struct {
		name      string
		pods      []*dbModel.GpuPods
		totalGPUs int32
		wantFully int
		wantPartial int
	}{
		{
			name:        "No pods",
			pods:        []*dbModel.GpuPods{},
			totalGPUs:   8,
			wantFully:   0,
			wantPartial: 0,
		},
		{
			name: "Mixed allocations",
			pods: []*dbModel.GpuPods{
				{GpuAllocated: 1},
				{GpuAllocated: 2},
				{GpuAllocated: 4},
			},
			totalGPUs:   8,
			wantFully:   1,
			wantPartial: 2,
		},
		{
			name: "All large allocations",
			pods: []*dbModel.GpuPods{
				{GpuAllocated: 4},
				{GpuAllocated: 4},
			},
			totalGPUs:   8,
			wantFully:   2,
			wantPartial: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAllocationPattern(tt.pods, tt.totalGPUs)
			if result.FullyAllocatedPods != tt.wantFully {
				t.Errorf("FullyAllocatedPods = %d; want %d",
					result.FullyAllocatedPods, tt.wantFully)
			}
			if result.PartiallyAllocPods != tt.wantPartial {
				t.Errorf("PartiallyAllocPods = %d; want %d",
					result.PartiallyAllocPods, tt.wantPartial)
			}
		})
	}
}

func TestCalculateLoadBalanceScore(t *testing.T) {
	tests := []struct {
		name      string
		nodeLoads []NodeLoad
		wantRange [2]float64 // min and max expected values
	}{
		{
			name:      "Empty nodes",
			nodeLoads: []NodeLoad{},
			wantRange: [2]float64{100, 100},
		},
		{
			name: "Perfect balance",
			nodeLoads: []NodeLoad{
				{AllocationRate: 50.0},
				{AllocationRate: 50.0},
				{AllocationRate: 50.0},
			},
			wantRange: [2]float64{100, 100},
		},
		{
			name: "High variance",
			nodeLoads: []NodeLoad{
				{AllocationRate: 10.0},
				{AllocationRate: 50.0},
				{AllocationRate: 90.0},
			},
			wantRange: [2]float64{0, 50},
		},
		{
			name: "Moderate variance",
			nodeLoads: []NodeLoad{
				{AllocationRate: 40.0},
				{AllocationRate: 50.0},
				{AllocationRate: 60.0},
			},
			wantRange: [2]float64{70, 100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateLoadBalanceScore(tt.nodeLoads)
			if result < tt.wantRange[0] || result > tt.wantRange[1] {
				t.Errorf("calculateLoadBalanceScore() = %f; want between %f and %f",
					result, tt.wantRange[0], tt.wantRange[1])
			}
		})
	}
}

