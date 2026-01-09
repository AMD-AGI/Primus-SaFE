// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package processtree

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInitCollector(t *testing.T) {
	ctx := context.Background()
	err := InitCollector(ctx)
	
	// May fail if containerd/kubelet are not available, but should not panic
	assert.NoError(t, err)
	
	collector := GetCollector()
	assert.NotNil(t, collector)
	assert.NotNil(t, collector.procReader)
	assert.NotNil(t, collector.nsenterExecutor)
	assert.Equal(t, 5*time.Minute, collector.cacheTTL)
}

func TestGetCollector(t *testing.T) {
	// Initialize collector first
	InitCollector(context.Background())
	
	collector := GetCollector()
	assert.NotNil(t, collector)
	
	// Should return the same instance
	collector2 := GetCollector()
	assert.Equal(t, collector, collector2)
}

func TestCacheKey(t *testing.T) {
	key1 := cacheKey{
		PodUID:           "pod-123",
		IncludeEnv:       true,
		IncludeCmdline:   true,
		IncludeResources: false,
	}

	key2 := cacheKey{
		PodUID:           "pod-123",
		IncludeEnv:       true,
		IncludeCmdline:   true,
		IncludeResources: false,
	}

	key3 := cacheKey{
		PodUID:           "pod-123",
		IncludeEnv:       false, // Different
		IncludeCmdline:   true,
		IncludeResources: false,
	}

	// Same keys should be equal
	assert.Equal(t, key1, key2)
	
	// Different keys should not be equal
	assert.NotEqual(t, key1, key3)
}

func TestInvalidateCache(t *testing.T) {
	InitCollector(context.Background())
	collector := GetCollector()

	// Store some test data in cache
	key1 := cacheKey{
		PodUID:         "pod-123",
		IncludeEnv:     true,
		IncludeCmdline: true,
	}

	key2 := cacheKey{
		PodUID:         "pod-456",
		IncludeEnv:     true,
		IncludeCmdline: true,
	}

	tree1 := &PodProcessTree{
		PodUID:      "pod-123",
		CollectedAt: time.Now(),
	}

	tree2 := &PodProcessTree{
		PodUID:      "pod-456",
		CollectedAt: time.Now(),
	}

	collector.cache.Store(key1, tree1)
	collector.cache.Store(key2, tree2)

	// Invalidate cache for pod-123
	collector.InvalidateCache("pod-123")

	// pod-123 should be removed
	_, ok := collector.cache.Load(key1)
	assert.False(t, ok)

	// pod-456 should still be there
	_, ok = collector.cache.Load(key2)
	assert.True(t, ok)
}

func TestProcessInfo_Classification(t *testing.T) {
	t.Run("python process", func(t *testing.T) {
		info := &ProcessInfo{
			Cmdline:  "python train.py --epochs 100",
			IsPython: true,
		}

		assert.True(t, info.IsPython)
		assert.False(t, info.IsJava)
	})

	t.Run("java process", func(t *testing.T) {
		info := &ProcessInfo{
			Cmdline: "java -jar application.jar",
			IsJava:  true,
		}

		assert.False(t, info.IsPython)
		assert.True(t, info.IsJava)
	})

	t.Run("other process", func(t *testing.T) {
		info := &ProcessInfo{
			Cmdline: "/bin/bash",
		}

		assert.False(t, info.IsPython)
		assert.False(t, info.IsJava)
	})
}

func TestContainerProcessTree_Counts(t *testing.T) {
	tree := &ContainerProcessTree{
		ContainerID:   "container-123",
		ContainerName: "test-container",
		AllProcesses: []*ProcessInfo{
			{HostPID: 1, IsPython: true},
			{HostPID: 2, IsPython: false},
			{HostPID: 3, IsPython: true},
		},
		ProcessCount: 3,
		PythonCount:  2,
	}

	assert.Equal(t, 3, tree.ProcessCount)
	assert.Equal(t, 2, tree.PythonCount)
	assert.Len(t, tree.AllProcesses, 3)
}

func TestPodProcessTree_Summary(t *testing.T) {
	container1 := &ContainerProcessTree{
		ContainerID:  "c1",
		ProcessCount: 5,
		PythonCount:  2,
	}

	container2 := &ContainerProcessTree{
		ContainerID:  "c2",
		ProcessCount: 3,
		PythonCount:  1,
	}

	tree := &PodProcessTree{
		PodUID:         "pod-123",
		PodName:        "test-pod",
		PodNamespace:   "default",
		Containers:     []*ContainerProcessTree{container1, container2},
		TotalProcesses: 8,
		TotalPython:    3,
		CollectedAt:    time.Now(),
	}

	assert.Equal(t, 8, tree.TotalProcesses)
	assert.Equal(t, 3, tree.TotalPython)
	assert.Len(t, tree.Containers, 2)
}

func TestProcessTreeRequest_Options(t *testing.T) {
	req := &ProcessTreeRequest{
		PodName:          "test-pod",
		PodNamespace:     "default",
		PodUID:           "pod-123",
		IncludeEnv:       true,
		IncludeCmdline:   true,
		IncludeResources: false,
	}

	assert.True(t, req.IncludeEnv)
	assert.True(t, req.IncludeCmdline)
	assert.False(t, req.IncludeResources)
}

func TestProcessState_Constants(t *testing.T) {
	assert.Equal(t, ProcessState("R"), ProcessStateRunning)
	assert.Equal(t, ProcessState("S"), ProcessStateSleeping)
	assert.Equal(t, ProcessState("D"), ProcessStateDiskSleep)
	assert.Equal(t, ProcessState("Z"), ProcessStateZombie)
	assert.Equal(t, ProcessState("T"), ProcessStateStopped)
	assert.Equal(t, ProcessState("t"), ProcessStateTracingStop)
	assert.Equal(t, ProcessState("X"), ProcessStateDead)
}

func TestTensorboardFileInfo_Fields(t *testing.T) {
	info := &TensorboardFileInfo{
		PID:      1234,
		FD:       "5",
		FilePath: "/var/log/tensorboard/events.out.tfevents.123",
		FileName: "events.out.tfevents.123",
	}

	assert.Equal(t, 1234, info.PID)
	assert.Equal(t, "5", info.FD)
	assert.Equal(t, "/var/log/tensorboard/events.out.tfevents.123", info.FilePath)
	assert.Equal(t, "events.out.tfevents.123", info.FileName)
}

func TestTensorboardFilesResponse_Fields(t *testing.T) {
	resp := &TensorboardFilesResponse{
		PodUID:       "pod-123",
		PodName:      "test-pod",
		PodNamespace: "default",
		Files: []*TensorboardFileInfo{
			{PID: 100, FileName: "events1.tfevents"},
			{PID: 200, FileName: "events2.tfevents"},
		},
		TotalProcesses: 10,
		CollectedAt:    time.Now(),
	}

	assert.Equal(t, "pod-123", resp.PodUID)
	assert.Equal(t, "test-pod", resp.PodName)
	assert.Equal(t, "default", resp.PodNamespace)
	assert.Len(t, resp.Files, 2)
	assert.Equal(t, 10, resp.TotalProcesses)
	assert.False(t, resp.CollectedAt.IsZero())
}

func TestProcessEnvInfo_Fields(t *testing.T) {
	info := &ProcessEnvInfo{
		PID:     1234,
		Cmdline: "python train.py",
		Environment: map[string]string{
			"PATH":       "/usr/bin",
			"PYTHONPATH": "/app",
		},
	}

	assert.Equal(t, 1234, info.PID)
	assert.Equal(t, "python train.py", info.Cmdline)
	assert.Len(t, info.Environment, 2)
	assert.Equal(t, "/usr/bin", info.Environment["PATH"])
	assert.Equal(t, "/app", info.Environment["PYTHONPATH"])
}

func TestProcessEnvRequest_Filter(t *testing.T) {
	req := &ProcessEnvRequest{
		PodUID:       "pod-123",
		PID:          1234,
		FilterPrefix: "CUDA_",
	}

	assert.Equal(t, "pod-123", req.PodUID)
	assert.Equal(t, 1234, req.PID)
	assert.Equal(t, "CUDA_", req.FilterPrefix)
}

func TestProcessArgInfo_Fields(t *testing.T) {
	info := &ProcessArgInfo{
		PID:     1234,
		Cmdline: "python train.py --epochs 100",
		Args:    []string{"python", "train.py", "--epochs", "100"},
	}

	assert.Equal(t, 1234, info.PID)
	assert.Equal(t, "python train.py --epochs 100", info.Cmdline)
	assert.Len(t, info.Args, 4)
	assert.Equal(t, "python", info.Args[0])
	assert.Equal(t, "100", info.Args[3])
}

func TestProcessArgsRequest_Fields(t *testing.T) {
	req := &ProcessArgsRequest{
		PodUID: "pod-123",
		PID:    1234,
	}

	assert.Equal(t, "pod-123", req.PodUID)
	assert.Equal(t, 1234, req.PID)
}

func TestProcessInfo_TreeStructure(t *testing.T) {
	// Create a simple process tree
	root := &ProcessInfo{
		HostPID:  1,
		HostPPID: 0,
		Comm:     "init",
		Children: []*ProcessInfo{},
	}

	child1 := &ProcessInfo{
		HostPID:  2,
		HostPPID: 1,
		Comm:     "child1",
	}

	child2 := &ProcessInfo{
		HostPID:  3,
		HostPPID: 1,
		Comm:     "child2",
	}

	root.Children = append(root.Children, child1, child2)

	assert.Equal(t, 1, root.HostPID)
	assert.Len(t, root.Children, 2)
	assert.Equal(t, "child1", root.Children[0].Comm)
	assert.Equal(t, "child2", root.Children[1].Comm)
}

func TestContainerInfo_Fields(t *testing.T) {
	info := &ContainerInfo{
		ID:    "abc123",
		Name:  "test-container",
		Image: "nginx:latest",
	}

	assert.Equal(t, "abc123", info.ID)
	assert.Equal(t, "test-container", info.Name)
	assert.Equal(t, "nginx:latest", info.Image)
}

// Integration test - only runs if collector is properly initialized
func TestGetPodProcessTree_NoContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if containerd is not available
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Test panicked (likely containerd not available): %v", r)
		}
	}()

	InitCollector(context.Background())
	collector := GetCollector()

	req := &ProcessTreeRequest{
		PodUID:         "nonexistent-pod-uid",
		PodName:        "nonexistent-pod",
		PodNamespace:   "default",
		IncludeCmdline: true,
	}

	_, err := collector.GetPodProcessTree(context.Background(), req)
	
	// Should fail because no containers found
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no containers found")
}

func TestFindPythonProcesses_NoProcesses(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Test panicked (likely containerd not available): %v", r)
		}
	}()

	InitCollector(context.Background())
	collector := GetCollector()

	_, err := collector.FindPythonProcesses(context.Background(), "nonexistent-pod-uid")
	
	// Should fail because pod not found
	assert.Error(t, err)
}

func TestGetProcessEnvironment_NoContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Test panicked (likely containerd not available): %v", r)
		}
	}()

	InitCollector(context.Background())
	collector := GetCollector()

	req := &ProcessEnvRequest{
		PodUID: "nonexistent-pod-uid",
	}

	_, err := collector.GetProcessEnvironment(context.Background(), req)
	
	// Should fail because no containers found
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no containers found")
}

func TestGetProcessArguments_NoContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Test panicked (likely containerd not available): %v", r)
		}
	}()

	InitCollector(context.Background())
	collector := GetCollector()

	req := &ProcessArgsRequest{
		PodUID: "nonexistent-pod-uid",
	}

	_, err := collector.GetProcessArguments(context.Background(), req)
	
	// Should fail because no containers found
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no containers found")
}

func TestFindTensorboardFiles_NoContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Test panicked (likely containerd not available): %v", r)
		}
	}()

	InitCollector(context.Background())
	collector := GetCollector()

	_, err := collector.FindTensorboardFiles(context.Background(), "nonexistent-pod-uid", "test-pod", "default")
	
	// Should fail because no containers found
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no containers found")
}

// Benchmark tests
func BenchmarkCacheKeyCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = cacheKey{
			PodUID:           "pod-123",
			IncludeEnv:       true,
			IncludeCmdline:   true,
			IncludeResources: false,
		}
	}
}

func BenchmarkInvalidateCache(b *testing.B) {
	InitCollector(context.Background())
	collector := GetCollector()

	// Pre-populate cache
	for i := 0; i < 100; i++ {
		key := cacheKey{
			PodUID:         "pod-123",
			IncludeEnv:     i%2 == 0,
			IncludeCmdline: i%3 == 0,
		}
		tree := &PodProcessTree{
			PodUID:      "pod-123",
			CollectedAt: time.Now(),
		}
		collector.cache.Store(key, tree)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector.InvalidateCache("pod-123")
	}
}

