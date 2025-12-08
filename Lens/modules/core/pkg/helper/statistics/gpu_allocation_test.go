package statistics

import (
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

func TestMaxTime(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)

	if result := maxTime(t1, t2); !result.Equal(t2) {
		t.Errorf("maxTime(%v, %v) = %v, want %v", t1, t2, result, t2)
	}

	if result := maxTime(t2, t1); !result.Equal(t2) {
		t.Errorf("maxTime(%v, %v) = %v, want %v", t2, t1, result, t2)
	}

	if result := maxTime(t1, t1); !result.Equal(t1) {
		t.Errorf("maxTime(%v, %v) = %v, want %v", t1, t1, result, t1)
	}
}

func TestCalculatePodAllocation(t *testing.T) {
	calculator := &GpuAllocationCalculator{}

	testCases := []struct {
		name           string
		pod            *model.GpuPods
		startTime      time.Time
		endTime        time.Time
		now            time.Time
		expectedActive float64
	}{
		{
			name: "Pod fully within time range",
			pod: &model.GpuPods{
				UID:          "pod-1",
				GpuAllocated: 4,
				CreatedAt:    time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
				UpdatedAt:    time.Date(2024, 1, 1, 10, 45, 0, 0, time.UTC),
				Phase:        "Succeeded",
			},
			startTime:      time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
			now:            time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
			expectedActive: 15 * 60, // 15 minutes = 900 seconds
		},
		{
			name: "Pod started before time range",
			pod: &model.GpuPods{
				UID:          "pod-2",
				GpuAllocated: 8,
				CreatedAt:    time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC),
				UpdatedAt:    time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
				Phase:        "Succeeded",
			},
			startTime:      time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
			now:            time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
			expectedActive: 30 * 60, // 30 minutes = 1800 seconds
		},
		{
			name: "Pod ended after time range",
			pod: &model.GpuPods{
				UID:          "pod-3",
				GpuAllocated: 2,
				CreatedAt:    time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
				UpdatedAt:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Phase:        "Succeeded",
			},
			startTime:      time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
			now:            time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			expectedActive: 30 * 60, // 30 minutes = 1800 seconds
		},
		{
			name: "Pod spans entire time range",
			pod: &model.GpuPods{
				UID:          "pod-4",
				GpuAllocated: 1,
				CreatedAt:    time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC),
				UpdatedAt:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Phase:        "Succeeded",
			},
			startTime:      time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
			now:            time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			expectedActive: 60 * 60, // 60 minutes = 3600 seconds
		},
		{
			name: "Pod still running",
			pod: &model.GpuPods{
				UID:          "pod-5",
				GpuAllocated: 4,
				CreatedAt:    time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
				UpdatedAt:    time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
				Phase:        "Running", // Running phase = still running
			},
			startTime:      time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
			now:            time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
			expectedActive: 30 * 60, // 30 minutes = 1800 seconds
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			totalDuration := tc.endTime.Sub(tc.startTime).Seconds()
			result := calculator.calculatePodAllocation(tc.pod, tc.startTime, tc.endTime, totalDuration, tc.now)

			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			if result.ActiveDuration != tc.expectedActive {
				t.Errorf("ActiveDuration = %v, want %v", result.ActiveDuration, tc.expectedActive)
			}

			if result.GpuCount != tc.pod.GpuAllocated {
				t.Errorf("GpuCount = %v, want %v", result.GpuCount, tc.pod.GpuAllocated)
			}
		})
	}
}

func TestCalculateWorkloadAllocation(t *testing.T) {
	calculator := &GpuAllocationCalculator{}

	startTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)
	now := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)
	totalDuration := endTime.Sub(startTime).Seconds() // 3600 seconds

	workload := &model.GpuWorkload{
		UID:       "workload-1",
		Name:      "test-job",
		Namespace: "default",
		Kind:      "Job",
		CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		EndAt:     time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
	}

	// Two pods: one runs first 30 min with 4 GPUs, another runs last 30 min with 8 GPUs
	pods := []*model.GpuPods{
		{
			UID:          "pod-1",
			GpuAllocated: 4,
			CreatedAt:    time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
			Phase:        "Succeeded",
		},
		{
			UID:          "pod-2",
			GpuAllocated: 8,
			CreatedAt:    time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
			Phase:        "Succeeded",
		},
	}

	result := calculator.calculateWorkloadAllocation(workload, pods, startTime, endTime, totalDuration, now)

	// Expected allocation: (4 * 1800/3600) + (8 * 1800/3600) = 2 + 4 = 6
	expectedAllocation := 6.0
	if result.AllocatedGpu != expectedAllocation {
		t.Errorf("AllocatedGpu = %v, want %v", result.AllocatedGpu, expectedAllocation)
	}

	if result.PodCount != 2 {
		t.Errorf("PodCount = %v, want 2", result.PodCount)
	}

	if result.WorkloadUID != "workload-1" {
		t.Errorf("WorkloadUID = %v, want workload-1", result.WorkloadUID)
	}
}

func TestCalculateWorkloadAllocation_NoOverlap(t *testing.T) {
	calculator := &GpuAllocationCalculator{}

	startTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)
	now := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)
	totalDuration := 3600.0 // 1 hour

	// Scenario: 100 GPU cluster, two non-overlapping workloads
	// Workload A: first 30 min, 80 GPUs
	// Workload B: last 30 min, 60 GPUs
	// Expected cluster allocation: (80 * 0.5) + (60 * 0.5) = 40 + 30 = 70 GPUs average
	// NOT 80 + 60 = 140 GPUs (which would be > 100%)

	workloadA := &model.GpuWorkload{
		UID:       "workload-a",
		Name:      "job-a",
		Namespace: "default",
		Kind:      "Job",
		CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		EndAt:     time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
	}

	workloadB := &model.GpuWorkload{
		UID:       "workload-b",
		Name:      "job-b",
		Namespace: "default",
		Kind:      "Job",
		CreatedAt: time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
		EndAt:     time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
	}

	podsA := []*model.GpuPods{
		{
			UID:          "pod-a",
			GpuAllocated: 80,
			CreatedAt:    time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
			Phase:        "Succeeded",
		},
	}

	podsB := []*model.GpuPods{
		{
			UID:          "pod-b",
			GpuAllocated: 60,
			CreatedAt:    time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
			Phase:        "Succeeded",
		},
	}

	resultA := calculator.calculateWorkloadAllocation(workloadA, podsA, startTime, endTime, totalDuration, now)
	resultB := calculator.calculateWorkloadAllocation(workloadB, podsB, startTime, endTime, totalDuration, now)

	// Workload A: 80 GPUs * 0.5 = 40 GPUs average
	expectedA := 40.0
	if resultA.AllocatedGpu != expectedA {
		t.Errorf("Workload A AllocatedGpu = %v, want %v", resultA.AllocatedGpu, expectedA)
	}

	// Workload B: 60 GPUs * 0.5 = 30 GPUs average
	expectedB := 30.0
	if resultB.AllocatedGpu != expectedB {
		t.Errorf("Workload B AllocatedGpu = %v, want %v", resultB.AllocatedGpu, expectedB)
	}

	// Total cluster allocation: 40 + 30 = 70 GPUs average
	totalAllocation := resultA.AllocatedGpu + resultB.AllocatedGpu
	expectedTotal := 70.0
	if totalAllocation != expectedTotal {
		t.Errorf("Total AllocatedGpu = %v, want %v", totalAllocation, expectedTotal)
	}

	// Verify it's NOT 140 (which would exceed 100% for a 100 GPU cluster)
	if totalAllocation > 100 {
		t.Errorf("Total allocation %v exceeds cluster capacity 100, time-weighted calculation failed", totalAllocation)
	}
}

func TestGpuAllocationResult_Empty(t *testing.T) {
	result := &GpuAllocationResult{}

	if result.TotalAllocatedGpu != 0 {
		t.Errorf("TotalAllocatedGpu = %v, want 0", result.TotalAllocatedGpu)
	}

	if result.WorkloadCount != 0 {
		t.Errorf("WorkloadCount = %v, want 0", result.WorkloadCount)
	}

	if result.PodCount != 0 {
		t.Errorf("PodCount = %v, want 0", result.PodCount)
	}
}

func TestWorkloadAllocationDetail_Fields(t *testing.T) {
	detail := WorkloadAllocationDetail{
		WorkloadUID:    "uid-123",
		WorkloadName:   "my-job",
		Namespace:      "ml-team",
		WorkloadKind:   "Job",
		AllocatedGpu:   8.5,
		ActiveDuration: 1800.0,
		PodCount:       3,
	}

	if detail.WorkloadUID != "uid-123" {
		t.Errorf("WorkloadUID = %v, want uid-123", detail.WorkloadUID)
	}

	if detail.Namespace != "ml-team" {
		t.Errorf("Namespace = %v, want ml-team", detail.Namespace)
	}

	if detail.AllocatedGpu != 8.5 {
		t.Errorf("AllocatedGpu = %v, want 8.5", detail.AllocatedGpu)
	}
}
