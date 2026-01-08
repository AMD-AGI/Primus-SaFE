package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessInfo_JSON(t *testing.T) {
	process := ProcessInfo{
		HostPID:       12345,
		HostPPID:      1,
		ContainerPID:  100,
		ContainerPPID: 1,
		Cmdline:       "/usr/bin/python3 train.py",
		Comm:          "python3",
		Exe:           "/usr/bin/python3",
		Args:          []string{"train.py", "--epochs=100"},
		State:         "R",
		Threads:       4,
		IsPython:      true,
		HasGPU:        true,
		GPUDevices: []GPUDeviceBinding{
			{DevicePath: "/dev/dri/renderD128", CardIndex: 0},
		},
	}

	jsonBytes, err := json.Marshal(process)
	require.NoError(t, err)

	var decoded ProcessInfo
	err = json.Unmarshal(jsonBytes, &decoded)
	require.NoError(t, err)

	assert.Equal(t, process.HostPID, decoded.HostPID)
	assert.Equal(t, process.Cmdline, decoded.Cmdline)
	assert.Equal(t, process.IsPython, decoded.IsPython)
	assert.Equal(t, process.HasGPU, decoded.HasGPU)
	assert.Len(t, decoded.GPUDevices, 1)
}

func TestGPUDeviceBinding_JSON(t *testing.T) {
	binding := GPUDeviceBinding{
		DevicePath: "/dev/dri/renderD128",
		CardIndex:  0,
		UUID:       "GPU-abc123",
		MarketName: "AMD Instinct MI300X",
		BDF:        "0000:03:00.0",
	}

	jsonBytes, err := json.Marshal(binding)
	require.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"device_path":"/dev/dri/renderD128"`)
	assert.Contains(t, string(jsonBytes), `"card_index":0`)
	assert.Contains(t, string(jsonBytes), `"bdf":"0000:03:00.0"`)
}

func TestContainerProcessTree_GetPythonProcesses(t *testing.T) {
	tests := []struct {
		name          string
		tree          *ContainerProcessTree
		expectedCount int
	}{
		{
			name: "no python processes",
			tree: &ContainerProcessTree{
				ContainerID: "container-1",
				RootProcess: &ProcessInfo{
					HostPID:  1,
					IsPython: false,
					Children: []*ProcessInfo{
						{HostPID: 2, IsPython: false},
						{HostPID: 3, IsPython: false},
					},
				},
			},
			expectedCount: 0,
		},
		{
			name: "root is python",
			tree: &ContainerProcessTree{
				ContainerID: "container-2",
				RootProcess: &ProcessInfo{
					HostPID:  1,
					IsPython: true,
				},
			},
			expectedCount: 1,
		},
		{
			name: "children are python",
			tree: &ContainerProcessTree{
				ContainerID: "container-3",
				RootProcess: &ProcessInfo{
					HostPID:  1,
					IsPython: false,
					Children: []*ProcessInfo{
						{HostPID: 2, IsPython: true},
						{HostPID: 3, IsPython: true},
						{HostPID: 4, IsPython: false},
					},
				},
			},
			expectedCount: 2,
		},
		{
			name: "nested python processes",
			tree: &ContainerProcessTree{
				ContainerID: "container-4",
				RootProcess: &ProcessInfo{
					HostPID:  1,
					IsPython: false,
					Children: []*ProcessInfo{
						{
							HostPID:  2,
							IsPython: true,
							Children: []*ProcessInfo{
								{HostPID: 3, IsPython: true},
							},
						},
					},
				},
			},
			expectedCount: 2,
		},
		{
			name: "nil root process",
			tree: &ContainerProcessTree{
				ContainerID: "container-5",
				RootProcess: nil,
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pythonProcesses := tt.tree.GetPythonProcesses()
			assert.Len(t, pythonProcesses, tt.expectedCount)
		})
	}
}

func TestContainerProcessTree_GetGPUProcesses(t *testing.T) {
	tests := []struct {
		name          string
		tree          *ContainerProcessTree
		expectedCount int
	}{
		{
			name: "no GPU processes",
			tree: &ContainerProcessTree{
				ContainerID: "container-1",
				RootProcess: &ProcessInfo{
					HostPID: 1,
					HasGPU:  false,
					Children: []*ProcessInfo{
						{HostPID: 2, HasGPU: false},
					},
				},
			},
			expectedCount: 0,
		},
		{
			name: "root has GPU",
			tree: &ContainerProcessTree{
				ContainerID: "container-2",
				RootProcess: &ProcessInfo{
					HostPID: 1,
					HasGPU:  true,
				},
			},
			expectedCount: 1,
		},
		{
			name: "multiple GPU processes",
			tree: &ContainerProcessTree{
				ContainerID: "container-3",
				RootProcess: &ProcessInfo{
					HostPID: 1,
					HasGPU:  true,
					Children: []*ProcessInfo{
						{HostPID: 2, HasGPU: true},
						{HostPID: 3, HasGPU: false},
						{
							HostPID: 4,
							HasGPU:  true,
							Children: []*ProcessInfo{
								{HostPID: 5, HasGPU: true},
							},
						},
					},
				},
			},
			expectedCount: 4,
		},
		{
			name: "nil root process",
			tree: &ContainerProcessTree{
				ContainerID: "container-4",
				RootProcess: nil,
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpuProcesses := tt.tree.GetGPUProcesses()
			assert.Len(t, gpuProcesses, tt.expectedCount)
		})
	}
}

func TestPodProcessTree_GetPythonProcesses(t *testing.T) {
	tree := &PodProcessTree{
		PodName:      "test-pod",
		PodNamespace: "default",
		PodUID:       "pod-123",
		Containers: []*ContainerProcessTree{
			{
				ContainerID: "container-1",
				RootProcess: &ProcessInfo{
					HostPID:  1,
					IsPython: true,
					Children: []*ProcessInfo{
						{HostPID: 2, IsPython: true},
					},
				},
			},
			{
				ContainerID: "container-2",
				RootProcess: &ProcessInfo{
					HostPID:  10,
					IsPython: false,
					Children: []*ProcessInfo{
						{HostPID: 11, IsPython: true},
					},
				},
			},
		},
	}

	pythonProcesses := tree.GetPythonProcesses()
	assert.Len(t, pythonProcesses, 3)
}

func TestPodProcessTree_GetGPUProcesses(t *testing.T) {
	tree := &PodProcessTree{
		PodName:      "test-pod",
		PodNamespace: "default",
		PodUID:       "pod-123",
		Containers: []*ContainerProcessTree{
			{
				ContainerID: "container-1",
				RootProcess: &ProcessInfo{
					HostPID: 1,
					HasGPU:  true,
				},
			},
			{
				ContainerID: "container-2",
				RootProcess: &ProcessInfo{
					HostPID: 10,
					HasGPU:  false,
					Children: []*ProcessInfo{
						{HostPID: 11, HasGPU: true},
						{HostPID: 12, HasGPU: true},
					},
				},
			},
		},
	}

	gpuProcesses := tree.GetGPUProcesses()
	assert.Len(t, gpuProcesses, 3)
}

func TestPodProcessTree_EmptyContainers(t *testing.T) {
	tree := &PodProcessTree{
		PodName:      "test-pod",
		PodNamespace: "default",
		PodUID:       "pod-123",
		Containers:   []*ContainerProcessTree{},
	}

	pythonProcesses := tree.GetPythonProcesses()
	assert.Len(t, pythonProcesses, 0)

	gpuProcesses := tree.GetGPUProcesses()
	assert.Len(t, gpuProcesses, 0)
}

func TestPodProcessTree_NilContainers(t *testing.T) {
	tree := &PodProcessTree{
		PodName:      "test-pod",
		PodNamespace: "default",
		PodUID:       "pod-123",
		Containers:   nil,
	}

	pythonProcesses := tree.GetPythonProcesses()
	assert.Len(t, pythonProcesses, 0)

	gpuProcesses := tree.GetGPUProcesses()
	assert.Len(t, gpuProcesses, 0)
}

func TestPodProcessTree_JSON(t *testing.T) {
	now := time.Now()
	tree := PodProcessTree{
		PodName:        "training-job-0",
		PodNamespace:   "ml-training",
		PodUID:         "pod-uid-123",
		NodeName:       "gpu-node-1",
		TotalProcesses: 10,
		TotalPython:    5,
		CollectedAt:    now,
		Containers: []*ContainerProcessTree{
			{
				ContainerID:   "container-abc",
				ContainerName: "trainer",
				ProcessCount:  10,
				PythonCount:   5,
			},
		},
	}

	jsonBytes, err := json.Marshal(tree)
	require.NoError(t, err)

	var decoded PodProcessTree
	err = json.Unmarshal(jsonBytes, &decoded)
	require.NoError(t, err)

	assert.Equal(t, tree.PodName, decoded.PodName)
	assert.Equal(t, tree.PodNamespace, decoded.PodNamespace)
	assert.Equal(t, tree.TotalProcesses, decoded.TotalProcesses)
	assert.Len(t, decoded.Containers, 1)
}

func TestProcessTreeRequest_Fields(t *testing.T) {
	req := ProcessTreeRequest{
		PodUID:           "pod-123",
		ContainerID:      "container-abc",
		IncludeCmdline:   true,
		IncludeEnv:       true,
		IncludeArgs:      true,
		IncludeGPU:       true,
		MaxDepth:         5,
		FilterPythonOnly: true,
	}

	assert.Equal(t, "pod-123", req.PodUID)
	assert.Equal(t, "container-abc", req.ContainerID)
	assert.True(t, req.IncludeCmdline)
	assert.True(t, req.IncludeEnv)
	assert.True(t, req.IncludeArgs)
	assert.True(t, req.IncludeGPU)
	assert.Equal(t, 5, req.MaxDepth)
	assert.True(t, req.FilterPythonOnly)
}

func TestProcessEnvRequest_JSON(t *testing.T) {
	req := ProcessEnvRequest{
		PodUID:      "pod-123",
		ContainerID: "container-abc",
		HostPID:     12345,
	}

	jsonBytes, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"pod_uid":"pod-123"`)
	assert.Contains(t, string(jsonBytes), `"host_pid":12345`)
}

func TestProcessEnvResponse_JSON(t *testing.T) {
	resp := ProcessEnvResponse{
		PodUID:      "pod-123",
		ContainerID: "container-abc",
		HostPID:     12345,
		Env:         []string{"PATH=/usr/bin", "HOME=/root", "CUDA_VISIBLE_DEVICES=0,1"},
	}

	jsonBytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded ProcessEnvResponse
	err = json.Unmarshal(jsonBytes, &decoded)
	require.NoError(t, err)

	assert.Equal(t, resp.PodUID, decoded.PodUID)
	assert.Len(t, decoded.Env, 3)
}

func TestProcessArgsRequest_JSON(t *testing.T) {
	req := ProcessArgsRequest{
		PodUID:      "pod-123",
		ContainerID: "container-abc",
		HostPID:     12345,
	}

	jsonBytes, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"pod_uid":"pod-123"`)
	assert.Contains(t, string(jsonBytes), `"host_pid":12345`)
}

func TestProcessArgsResponse_JSON(t *testing.T) {
	resp := ProcessArgsResponse{
		PodUID:      "pod-123",
		ContainerID: "container-abc",
		HostPID:     12345,
		Args:        []string{"/usr/bin/python3", "train.py", "--epochs=100"},
	}

	jsonBytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded ProcessArgsResponse
	err = json.Unmarshal(jsonBytes, &decoded)
	require.NoError(t, err)

	assert.Equal(t, resp.PodUID, decoded.PodUID)
	assert.Len(t, decoded.Args, 3)
}

func TestContainerProcessTree_AllProcessesNotSerialized(t *testing.T) {
	tree := ContainerProcessTree{
		ContainerID:   "container-123",
		ContainerName: "trainer",
		AllProcesses: []*ProcessInfo{
			{HostPID: 1},
			{HostPID: 2},
			{HostPID: 3},
		},
		ProcessCount: 3,
	}

	jsonBytes, err := json.Marshal(tree)
	require.NoError(t, err)

	// AllProcesses should not be in JSON due to json:"-"
	assert.NotContains(t, string(jsonBytes), `"all_processes"`)
	assert.Contains(t, string(jsonBytes), `"container_id":"container-123"`)
}

func TestProcessInfo_ResourceUsage(t *testing.T) {
	process := ProcessInfo{
		HostPID:       12345,
		CPUTime:       10000,
		MemoryRSS:     1024 * 1024 * 100, // 100 MB
		MemoryVirtual: 1024 * 1024 * 500, // 500 MB
	}

	assert.Equal(t, uint64(10000), process.CPUTime)
	assert.Equal(t, uint64(1024*1024*100), process.MemoryRSS)
	assert.Equal(t, uint64(1024*1024*500), process.MemoryVirtual)
}

func TestProcessInfo_ContainerContext(t *testing.T) {
	process := ProcessInfo{
		HostPID:       12345,
		ContainerID:   "docker://abc123",
		ContainerName: "trainer",
		PodUID:        "pod-uid-123",
		PodName:       "training-job-0",
		PodNamespace:  "ml-training",
	}

	assert.Equal(t, "docker://abc123", process.ContainerID)
	assert.Equal(t, "trainer", process.ContainerName)
	assert.Equal(t, "pod-uid-123", process.PodUID)
	assert.Equal(t, "training-job-0", process.PodName)
	assert.Equal(t, "ml-training", process.PodNamespace)
}

func BenchmarkPodProcessTree_GetPythonProcesses(b *testing.B) {
	// Create a tree with many processes
	children := make([]*ProcessInfo, 100)
	for i := 0; i < 100; i++ {
		children[i] = &ProcessInfo{
			HostPID:  i + 2,
			IsPython: i%3 == 0, // Every 3rd process is Python
		}
	}

	tree := &PodProcessTree{
		Containers: []*ContainerProcessTree{
			{
				RootProcess: &ProcessInfo{
					HostPID:  1,
					IsPython: true,
					Children: children,
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tree.GetPythonProcesses()
	}
}

func BenchmarkPodProcessTree_GetGPUProcesses(b *testing.B) {
	// Create a tree with many processes
	children := make([]*ProcessInfo, 100)
	for i := 0; i < 100; i++ {
		children[i] = &ProcessInfo{
			HostPID: i + 2,
			HasGPU:  i%5 == 0, // Every 5th process has GPU
		}
	}

	tree := &PodProcessTree{
		Containers: []*ContainerProcessTree{
			{
				RootProcess: &ProcessInfo{
					HostPID:  1,
					HasGPU:   true,
					Children: children,
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tree.GetGPUProcesses()
	}
}
