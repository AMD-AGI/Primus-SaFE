// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "Item exists in slice",
			slice:    []string{"pytorch", "tensorflow", "jax"},
			item:     "pytorch",
			expected: true,
		},
		{
			name:     "Item does not exist in slice",
			slice:    []string{"pytorch", "tensorflow"},
			item:     "jax",
			expected: false,
		},
		{
			name:     "Empty slice",
			slice:    []string{},
			item:     "pytorch",
			expected: false,
		},
		{
			name:     "Empty item in non-empty slice",
			slice:    []string{"pytorch", "tensorflow"},
			item:     "",
			expected: false,
		},
		{
			name:     "Empty item in slice with empty string",
			slice:    []string{"", "pytorch"},
			item:     "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("contains(%v, %q) = %v, want %v", tt.slice, tt.item, result, tt.expected)
			}
		})
	}
}

func TestCollector_findPythonProcessesInTree(t *testing.T) {
	c := &Collector{}

	tests := []struct {
		name     string
		root     *types.ProcessInfo
		expected int
	}{
		{
			name:     "Nil root",
			root:     nil,
			expected: 0,
		},
		{
			name: "Single Python process",
			root: &types.ProcessInfo{
				HostPID:  1000,
				IsPython: true,
				Cmdline:  "python script.py",
			},
			expected: 1,
		},
		{
			name: "No Python processes",
			root: &types.ProcessInfo{
				HostPID:  1000,
				IsPython: false,
				Cmdline:  "/bin/bash",
			},
			expected: 0,
		},
		{
			name: "Python process with children",
			root: &types.ProcessInfo{
				HostPID:  1000,
				IsPython: true,
				Cmdline:  "python script.py",
				Children: []*types.ProcessInfo{
					{
						HostPID:  1001,
						IsPython: true,
						Cmdline:  "python child.py",
					},
					{
						HostPID:  1002,
						IsPython: false,
						Cmdline:  "/bin/ls",
					},
				},
			},
			expected: 2,
		},
		{
			name: "Deep tree with multiple Python processes",
			root: &types.ProcessInfo{
				HostPID:  1000,
				IsPython: false,
				Cmdline:  "/bin/bash",
				Children: []*types.ProcessInfo{
					{
						HostPID:  1001,
						IsPython: true,
						Cmdline:  "python train.py",
						Children: []*types.ProcessInfo{
							{
								HostPID:  1002,
								IsPython: true,
								Cmdline:  "python worker.py",
							},
						},
					},
					{
						HostPID:  1003,
						IsPython: true,
						Cmdline:  "python monitor.py",
					},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.findPythonProcessesInTree(tt.root)
			if len(result) != tt.expected {
				t.Errorf("findPythonProcessesInTree() returned %d processes, want %d", len(result), tt.expected)
			}
		})
	}
}

func TestCollector_calculateProcessDepth(t *testing.T) {
	c := &Collector{}

	root := &types.ProcessInfo{
		HostPID:  1000,
		IsPython: false,
		Cmdline:  "/bin/bash",
		Children: []*types.ProcessInfo{
			{
				HostPID:  1001,
				IsPython: true,
				Cmdline:  "python train.py",
				Children: []*types.ProcessInfo{
					{
						HostPID:  1002,
						IsPython: true,
						Cmdline:  "python worker.py",
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		target   *types.ProcessInfo
		root     *types.ProcessInfo
		expected int
	}{
		{
			name:     "Nil root",
			target:   &types.ProcessInfo{HostPID: 1000},
			root:     nil,
			expected: -1,
		},
		{
			name:     "Target is root",
			target:   root,
			root:     root,
			expected: 0,
		},
		{
			name:     "Target is child",
			target:   root.Children[0],
			root:     root,
			expected: 1,
		},
		{
			name:     "Target is grandchild",
			target:   root.Children[0].Children[0],
			root:     root,
			expected: 2,
		},
		{
			name:     "Target not in tree",
			target:   &types.ProcessInfo{HostPID: 9999},
			root:     root,
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.calculateProcessDepth(tt.target, tt.root)
			if result != tt.expected {
				t.Errorf("calculateProcessDepth() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestCollector_detectPrimusFromProcesses(t *testing.T) {
	c := &Collector{}

	tests := []struct {
		name      string
		processes []*types.ProcessInfo
		wantNil   bool
	}{
		{
			name:      "Empty processes",
			processes: []*types.ProcessInfo{},
			wantNil:   true,
		},
		{
			name: "No Primus indicator",
			processes: []*types.ProcessInfo{
				{
					HostPID:  1000,
					Cmdline:  "python train.py --batch-size 32",
					IsPython: true,
				},
			},
			wantNil: true,
		},
		{
			name: "Primus in cmdline",
			processes: []*types.ProcessInfo{
				{
					HostPID:  1000,
					Cmdline:  "python train.py --primus --batch-size 32",
					IsPython: true,
				},
			},
			wantNil: false,
		},
		{
			name: "primus-train in cmdline",
			processes: []*types.ProcessInfo{
				{
					HostPID:  1000,
					Cmdline:  "primus-train --config config.yaml",
					IsPython: true,
				},
			},
			wantNil: false,
		},
		{
			name: "primus. module in cmdline",
			processes: []*types.ProcessInfo{
				{
					HostPID:  1000,
					Cmdline:  "python -m primus.train",
					IsPython: true,
				},
			},
			wantNil: false,
		},
		{
			name: "Primus with version flag",
			processes: []*types.ProcessInfo{
				{
					HostPID:  1000,
					Cmdline:  "python train.py --primus --version 1.0.0",
					IsPython: true,
				},
			},
			wantNil: false,
		},
		{
			name: "Case insensitive detection",
			processes: []*types.ProcessInfo{
				{
					HostPID:  1000,
					Cmdline:  "python train.py --PRIMUS",
					IsPython: true,
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.detectPrimusFromProcesses(tt.processes)
			if tt.wantNil && result != nil {
				t.Errorf("detectPrimusFromProcesses() = %v, want nil", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("detectPrimusFromProcesses() = nil, want non-nil")
			}
			if !tt.wantNil && result != nil {
				if result.Mode != "training" {
					t.Errorf("PrimusMetadata.Mode = %v, want training", result.Mode)
				}
			}
		})
	}
}

func TestCollector_findRootPythonProcesses(t *testing.T) {
	c := &Collector{}

	tests := []struct {
		name        string
		processTree *types.PodProcessTree
		expected    int
	}{
		{
			name: "Single container with one Python process",
			processTree: &types.PodProcessTree{
				TotalProcesses: 2,
				TotalPython:    1,
				Containers: []*types.ContainerProcessTree{
					{
						ContainerName: "main",
						RootProcess: &types.ProcessInfo{
							HostPID:  1000,
							IsPython: false,
							Children: []*types.ProcessInfo{
								{
									HostPID:  1001,
									IsPython: true,
									Cmdline:  "python train.py",
								},
							},
						},
					},
				},
			},
			expected: 1,
		},
		{
			name: "No Python processes",
			processTree: &types.PodProcessTree{
				TotalProcesses: 2,
				TotalPython:    0,
				Containers: []*types.ContainerProcessTree{
					{
						ContainerName: "main",
						RootProcess: &types.ProcessInfo{
							HostPID:  1000,
							IsPython: false,
							Children: []*types.ProcessInfo{
								{
									HostPID:  1001,
									IsPython: false,
								},
							},
						},
					},
				},
			},
			expected: 0,
		},
		{
			name: "Multiple Python processes at same depth",
			processTree: &types.PodProcessTree{
				TotalProcesses: 3,
				TotalPython:    2,
				Containers: []*types.ContainerProcessTree{
					{
						ContainerName: "main",
						RootProcess: &types.ProcessInfo{
							HostPID:  1000,
							IsPython: false,
							Children: []*types.ProcessInfo{
								{
									HostPID:  1001,
									IsPython: true,
									Cmdline:  "python train.py",
								},
								{
									HostPID:  1002,
									IsPython: true,
									Cmdline:  "python monitor.py",
								},
							},
						},
					},
				},
			},
			expected: 2,
		},
		{
			name: "Python at different depths - should return shallowest",
			processTree: &types.PodProcessTree{
				TotalProcesses: 3,
				TotalPython:    2,
				Containers: []*types.ContainerProcessTree{
					{
						ContainerName: "main",
						RootProcess: &types.ProcessInfo{
							HostPID:  1000,
							IsPython: false,
							Children: []*types.ProcessInfo{
								{
									HostPID:  1001,
									IsPython: true,
									Cmdline:  "python train.py",
									Children: []*types.ProcessInfo{
										{
											HostPID:  1002,
											IsPython: true,
											Cmdline:  "python worker.py",
										},
									},
								},
							},
						},
					},
				},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.findRootPythonProcesses(tt.processTree)
			if len(result) != tt.expected {
				t.Errorf("findRootPythonProcesses() returned %d processes, want %d", len(result), tt.expected)
			}
		})
	}
}

func TestCollector_InvalidateCache(t *testing.T) {
	storage := NewMockStorage()
	c := &Collector{
		storage:  storage,
		cacheTTL: 10 * time.Minute,
	}

	workloadUID := "test-workload-123"
	result := &CollectionResult{
		Success:  true,
		Duration: 1.0,
	}

	c.cache.Store(workloadUID, result)

	if _, ok := c.cache.Load(workloadUID); !ok {
		t.Fatal("Cache should contain the workload")
	}

	c.InvalidateCache(workloadUID)

	if _, ok := c.cache.Load(workloadUID); ok {
		t.Error("Cache should not contain the workload after invalidation")
	}
}

func TestCollector_GetMetadata(t *testing.T) {
	storage := NewMockStorage()
	c := &Collector{
		storage: storage,
	}

	ctx := context.Background()
	workloadUID := "test-workload-123"

	metadata := &WorkloadMetadata{
		WorkloadUID:  workloadUID,
		PodName:      "test-pod",
		PodNamespace: "default",
		Frameworks:   []string{"pytorch"},
	}

	err := storage.Store(ctx, metadata)
	if err != nil {
		t.Fatalf("Failed to store metadata: %v", err)
	}

	retrieved, err := c.GetMetadata(ctx, workloadUID)
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetMetadata() returned nil")
	}

	if retrieved.WorkloadUID != workloadUID {
		t.Errorf("WorkloadUID = %v, want %v", retrieved.WorkloadUID, workloadUID)
	}
}

func TestCollector_QueryMetadata(t *testing.T) {
	storage := NewMockStorage()
	c := &Collector{
		storage: storage,
	}

	ctx := context.Background()

	metadata1 := &WorkloadMetadata{
		WorkloadUID:   "workload-1",
		BaseFramework: "pytorch",
	}
	metadata2 := &WorkloadMetadata{
		WorkloadUID:   "workload-2",
		BaseFramework: "tensorflow",
	}

	storage.Store(ctx, metadata1)
	storage.Store(ctx, metadata2)

	query := &MetadataQuery{
		Framework: "pytorch",
	}

	results, err := c.QueryMetadata(ctx, query)
	if err != nil {
		t.Fatalf("QueryMetadata() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("QueryMetadata() returned %d results, want 1", len(results))
	}

	if len(results) > 0 && results[0].WorkloadUID != "workload-1" {
		t.Errorf("WorkloadUID = %v, want workload-1", results[0].WorkloadUID)
	}
}

