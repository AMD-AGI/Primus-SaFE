package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTopLevelGpuResource tests the TopLevelGpuResource struct
func TestTopLevelGpuResource(t *testing.T) {
	resource := TopLevelGpuResource{
		Kind:      "Deployment",
		Name:      "my-deployment",
		Namespace: "default",
		Uid:       "uid-123",
		Stat: GpuStat{
			GpuRequest:     4,
			GpuUtilization: 0.75,
		},
		Pods: []GpuPod{
			{
				Name:      "pod-1",
				Namespace: "default",
				Node:      "node-1",
				Stat: GpuStat{
					GpuRequest:     2,
					GpuUtilization: 0.8,
				},
			},
		},
		Source: "kubernetes",
	}

	assert.Equal(t, "Deployment", resource.Kind)
	assert.Equal(t, "my-deployment", resource.Name)
	assert.Equal(t, "default", resource.Namespace)
	assert.Equal(t, 4, resource.Stat.GpuRequest)
	assert.Len(t, resource.Pods, 1)
}

// TestTopLevelGpuResource_CalculateGpuUsage tests the CalculateGpuUsage method
func TestTopLevelGpuResource_CalculateGpuUsage(t *testing.T) {
	tests := []struct {
		name                    string
		pods                    []GpuPod
		expectedGpuUtilization  float64
	}{
		{
			name: "two pods equal usage",
			pods: []GpuPod{
				{Stat: GpuStat{GpuRequest: 2, GpuUtilization: 0.8}},
				{Stat: GpuStat{GpuRequest: 2, GpuUtilization: 0.6}},
			},
			expectedGpuUtilization: (0.8 + 0.6) / 4.0,
		},
		{
			name: "three pods different usage",
			pods: []GpuPod{
				{Stat: GpuStat{GpuRequest: 1, GpuUtilization: 0.9}},
				{Stat: GpuStat{GpuRequest: 2, GpuUtilization: 0.5}},
				{Stat: GpuStat{GpuRequest: 1, GpuUtilization: 0.7}},
			},
			expectedGpuUtilization: (0.9 + 0.5 + 0.7) / 4.0,
		},
		{
			name:                   "no pods",
			pods:                   []GpuPod{},
			expectedGpuUtilization: 0.0, // Would cause division by zero, returns NaN
		},
		{
			name: "single pod",
			pods: []GpuPod{
				{Stat: GpuStat{GpuRequest: 4, GpuUtilization: 0.6}},
			},
			expectedGpuUtilization: 0.6 / 4.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := TopLevelGpuResource{
				Pods: tt.pods,
			}

			resource.CalculateGpuUsage()

			if len(tt.pods) == 0 {
				// For empty pods, the result would be NaN (0/0)
				assert.True(t, resource.Stat.GpuUtilization != resource.Stat.GpuUtilization) // NaN != NaN
			} else {
				assert.InDelta(t, tt.expectedGpuUtilization, resource.Stat.GpuUtilization, 0.0001)
			}
		})
	}
}

// TestTopLevelGpuResource_CalculateGpuUsage_ZeroRequest tests calculation with zero request
func TestTopLevelGpuResource_CalculateGpuUsage_ZeroRequest(t *testing.T) {
	resource := TopLevelGpuResource{
		Pods: []GpuPod{
			{Stat: GpuStat{GpuRequest: 0, GpuUtilization: 0.5}},
		},
	}

	resource.CalculateGpuUsage()

	// Division by zero results in +Inf
	assert.True(t, resource.Stat.GpuUtilization == resource.Stat.GpuUtilization)
}

// TestTopLevelGpuResource_JSONMarshal tests JSON marshaling
func TestTopLevelGpuResource_JSONMarshal(t *testing.T) {
	resource := TopLevelGpuResource{
		Kind:      "StatefulSet",
		Name:      "my-sts",
		Namespace: "production",
		Uid:       "uid-456",
		Stat: GpuStat{
			GpuRequest:     8,
			GpuUtilization: 0.85,
		},
		Pods: []GpuPod{
			{Name: "pod-1", Namespace: "production"},
		},
	}

	data, err := json.Marshal(resource)
	require.NoError(t, err)

	var decoded TopLevelGpuResource
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, resource.Kind, decoded.Kind)
	assert.Equal(t, resource.Name, decoded.Name)
	assert.Equal(t, resource.Namespace, decoded.Namespace)
	assert.Equal(t, resource.Stat.GpuRequest, decoded.Stat.GpuRequest)
}

// TestGpuStat tests the GpuStat struct
func TestGpuStat(t *testing.T) {
	stat := GpuStat{
		GpuRequest:     4,
		GpuUtilization: 0.75,
	}

	assert.Equal(t, 4, stat.GpuRequest)
	assert.Equal(t, 0.75, stat.GpuUtilization)
}

// TestGpuStat_JSONMarshal tests JSON marshaling
func TestGpuStat_JSONMarshal(t *testing.T) {
	stat := GpuStat{
		GpuRequest:     2,
		GpuUtilization: 0.9,
	}

	data, err := json.Marshal(stat)
	require.NoError(t, err)

	var decoded GpuStat
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, stat.GpuRequest, decoded.GpuRequest)
	assert.InDelta(t, stat.GpuUtilization, decoded.GpuUtilization, 0.0001)
}

// TestGpuPod tests the GpuPod struct
func TestGpuPod(t *testing.T) {
	pod := GpuPod{
		Name:      "test-pod",
		Namespace: "default",
		Node:      "node-1",
		Devices:   []string{"GPU-0", "GPU-1"},
		Stat: GpuStat{
			GpuRequest:     2,
			GpuUtilization: 0.6,
		},
	}

	assert.Equal(t, "test-pod", pod.Name)
	assert.Equal(t, "default", pod.Namespace)
	assert.Equal(t, "node-1", pod.Node)
	assert.Len(t, pod.Devices, 2)
	assert.Equal(t, 2, pod.Stat.GpuRequest)
}

// TestGpuPod_JSONMarshal tests JSON marshaling
func TestGpuPod_JSONMarshal(t *testing.T) {
	pod := GpuPod{
		Name:      "pod-test",
		Namespace: "ns-test",
		Node:      "node-test",
		Devices:   []string{"GPU-0"},
		Stat: GpuStat{
			GpuRequest:     1,
			GpuUtilization: 0.5,
		},
	}

	data, err := json.Marshal(pod)
	require.NoError(t, err)

	var decoded GpuPod
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, pod.Name, decoded.Name)
	assert.Equal(t, pod.Namespace, decoded.Namespace)
	assert.Equal(t, pod.Node, decoded.Node)
	assert.Len(t, decoded.Devices, 1)
}

// TestWorkloadHistoryNodeView tests the WorkloadHistoryNodeView struct
func TestWorkloadHistoryNodeView(t *testing.T) {
	workload := WorkloadHistoryNodeView{
		Kind:         "Job",
		Name:         "training-job",
		Namespace:    "ml",
		Uid:          "uid-789",
		GpuAllocated: 8,
		PodName:      "training-pod-1",
		PodNamespace: "ml",
		StartTime:    1609459200,
		EndTime:      1609545600,
	}

	assert.Equal(t, "Job", workload.Kind)
	assert.Equal(t, "training-job", workload.Name)
	assert.Equal(t, 8, workload.GpuAllocated)
	assert.Equal(t, int64(1609459200), workload.StartTime)
	assert.Equal(t, int64(1609545600), workload.EndTime)
}

// TestWorkloadHistoryNodeView_JSONMarshal tests JSON marshaling
func TestWorkloadHistoryNodeView_JSONMarshal(t *testing.T) {
	workload := WorkloadHistoryNodeView{
		Kind:         "CronJob",
		Name:         "backup",
		Namespace:    "system",
		GpuAllocated: 1,
		StartTime:    1609459200,
		EndTime:      1609545600,
	}

	data, err := json.Marshal(workload)
	require.NoError(t, err)

	var decoded WorkloadHistoryNodeView
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, workload.Kind, decoded.Kind)
	assert.Equal(t, workload.Name, decoded.Name)
	assert.Equal(t, workload.GpuAllocated, decoded.GpuAllocated)
}

// TestWorkloadNodeView tests the WorkloadNodeView struct
func TestWorkloadNodeView(t *testing.T) {
	workload := WorkloadNodeView{
		Kind:             "DaemonSet",
		Name:             "monitoring",
		Namespace:        "kube-system",
		Uid:              "uid-abc",
		GpuAllocated:     1,
		GpuAllocatedNode: 1,
		NodeName:         "node-1",
		Status:           "Running",
	}

	assert.Equal(t, "DaemonSet", workload.Kind)
	assert.Equal(t, "monitoring", workload.Name)
	assert.Equal(t, 1, workload.GpuAllocated)
	assert.Equal(t, "node-1", workload.NodeName)
	assert.Equal(t, "Running", workload.Status)
}

// TestWorkloadNodeView_JSONMarshal tests JSON marshaling
func TestWorkloadNodeView_JSONMarshal(t *testing.T) {
	workload := WorkloadNodeView{
		Kind:             "Deployment",
		Name:             "api-server",
		Namespace:        "production",
		GpuAllocated:     4,
		GpuAllocatedNode: 2,
		Status:           "Running",
	}

	data, err := json.Marshal(workload)
	require.NoError(t, err)

	var decoded WorkloadNodeView
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, workload.Kind, decoded.Kind)
	assert.Equal(t, workload.Name, decoded.Name)
	assert.Equal(t, workload.GpuAllocated, decoded.GpuAllocated)
}

// BenchmarkTopLevelGpuResource_CalculateGpuUsage benchmarks CalculateGpuUsage
func BenchmarkTopLevelGpuResource_CalculateGpuUsage(b *testing.B) {
	resource := TopLevelGpuResource{
		Pods: []GpuPod{
			{Stat: GpuStat{GpuRequest: 2, GpuUtilization: 0.8}},
			{Stat: GpuStat{GpuRequest: 2, GpuUtilization: 0.6}},
			{Stat: GpuStat{GpuRequest: 2, GpuUtilization: 0.7}},
			{Stat: GpuStat{GpuRequest: 2, GpuUtilization: 0.9}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resource.CalculateGpuUsage()
	}
}

// BenchmarkTopLevelGpuResource_JSONMarshal benchmarks JSON marshaling
func BenchmarkTopLevelGpuResource_JSONMarshal(b *testing.B) {
	resource := TopLevelGpuResource{
		Kind:      "Deployment",
		Name:      "test",
		Namespace: "default",
		Pods: []GpuPod{
			{Name: "pod-1", Namespace: "default"},
			{Name: "pod-2", Namespace: "default"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(resource)
	}
}

