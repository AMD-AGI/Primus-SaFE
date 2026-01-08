package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPySpyOutputFormat_Constants(t *testing.T) {
	assert.Equal(t, PySpyOutputFormat("flamegraph"), PySpyFormatFlamegraph)
	assert.Equal(t, PySpyOutputFormat("speedscope"), PySpyFormatSpeedscope)
	assert.Equal(t, PySpyOutputFormat("raw"), PySpyFormatRaw)
}

func TestPySpySampleRequest_SetDefaults(t *testing.T) {
	tests := []struct {
		name     string
		req      PySpySampleRequest
		expected PySpySampleRequest
	}{
		{
			name: "empty request should have defaults",
			req:  PySpySampleRequest{},
			expected: PySpySampleRequest{
				Duration: 30,
				Rate:     100,
				Format:   string(PySpyFormatFlamegraph),
			},
		},
		{
			name: "custom values should be preserved",
			req: PySpySampleRequest{
				Duration: 60,
				Rate:     200,
				Format:   "speedscope",
			},
			expected: PySpySampleRequest{
				Duration: 60,
				Rate:     200,
				Format:   "speedscope",
			},
		},
		{
			name: "zero duration should use default",
			req: PySpySampleRequest{
				Duration: 0,
				Rate:     150,
				Format:   "raw",
			},
			expected: PySpySampleRequest{
				Duration: 30,
				Rate:     150,
				Format:   "raw",
			},
		},
		{
			name: "negative duration should use default",
			req: PySpySampleRequest{
				Duration: -10,
				Rate:     100,
				Format:   "flamegraph",
			},
			expected: PySpySampleRequest{
				Duration: 30,
				Rate:     100,
				Format:   "flamegraph",
			},
		},
		{
			name: "zero rate should use default",
			req: PySpySampleRequest{
				Duration: 60,
				Rate:     0,
				Format:   "flamegraph",
			},
			expected: PySpySampleRequest{
				Duration: 60,
				Rate:     100,
				Format:   "flamegraph",
			},
		},
		{
			name: "negative rate should use default",
			req: PySpySampleRequest{
				Duration: 60,
				Rate:     -50,
				Format:   "flamegraph",
			},
			expected: PySpySampleRequest{
				Duration: 60,
				Rate:     100,
				Format:   "flamegraph",
			},
		},
		{
			name: "empty format should use default",
			req: PySpySampleRequest{
				Duration: 45,
				Rate:     150,
				Format:   "",
			},
			expected: PySpySampleRequest{
				Duration: 45,
				Rate:     150,
				Format:   "flamegraph",
			},
		},
		{
			name: "all values set should preserve all",
			req: PySpySampleRequest{
				PodUID:       "test-pod-uid",
				PodName:      "test-pod",
				PodNamespace: "default",
				NodeName:     "node-1",
				PID:          12345,
				Duration:     120,
				Rate:         500,
				Format:       "speedscope",
				Native:       true,
				SubProcesses: true,
			},
			expected: PySpySampleRequest{
				PodUID:       "test-pod-uid",
				PodName:      "test-pod",
				PodNamespace: "default",
				NodeName:     "node-1",
				PID:          12345,
				Duration:     120,
				Rate:         500,
				Format:       "speedscope",
				Native:       true,
				SubProcesses: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.SetDefaults()
			assert.Equal(t, tt.expected.Duration, tt.req.Duration)
			assert.Equal(t, tt.expected.Rate, tt.req.Rate)
			assert.Equal(t, tt.expected.Format, tt.req.Format)
			assert.Equal(t, tt.expected.Native, tt.req.Native)
			assert.Equal(t, tt.expected.SubProcesses, tt.req.SubProcesses)
		})
	}
}

func TestPySpyTaskExt_JSON(t *testing.T) {
	ext := PySpyTaskExt{
		TaskID:         "pyspy-test123",
		TargetNodeName: "node-1",
		PodUID:         "pod-uid-123",
		PodName:        "test-pod",
		PodNamespace:   "default",
		HostPID:        12345,
		ContainerPID:   100,
		Duration:       30,
		Rate:           100,
		Format:         "flamegraph",
		Native:         true,
		SubProcesses:   false,
		OutputFile:     "/var/lib/lens/pyspy/test.svg",
		FileSize:       1024,
		StartedAt:      "2024-01-01T10:00:00Z",
		CompletedAt:    "2024-01-01T10:00:30Z",
	}

	// Test JSON marshaling
	jsonBytes, err := json.Marshal(ext)
	require.NoError(t, err)

	// Test JSON unmarshaling
	var decoded PySpyTaskExt
	err = json.Unmarshal(jsonBytes, &decoded)
	require.NoError(t, err)

	assert.Equal(t, ext.TaskID, decoded.TaskID)
	assert.Equal(t, ext.TargetNodeName, decoded.TargetNodeName)
	assert.Equal(t, ext.PodUID, decoded.PodUID)
	assert.Equal(t, ext.HostPID, decoded.HostPID)
	assert.Equal(t, ext.Duration, decoded.Duration)
	assert.Equal(t, ext.Rate, decoded.Rate)
	assert.Equal(t, ext.Format, decoded.Format)
	assert.Equal(t, ext.Native, decoded.Native)
	assert.Equal(t, ext.SubProcesses, decoded.SubProcesses)
	assert.Equal(t, ext.OutputFile, decoded.OutputFile)
	assert.Equal(t, ext.FileSize, decoded.FileSize)
}

func TestPySpyCompatibility_Fields(t *testing.T) {
	compat := PySpyCompatibility{
		Supported:       true,
		PythonProcesses: []int{123, 456, 789},
		Capabilities:    []string{"CAP_SYS_PTRACE"},
		CheckedAt:       time.Now(),
	}

	assert.True(t, compat.Supported)
	assert.Len(t, compat.PythonProcesses, 3)
	assert.Contains(t, compat.Capabilities, "CAP_SYS_PTRACE")
	assert.False(t, compat.CheckedAt.IsZero())
}

func TestPySpyCheckRequest_JSON(t *testing.T) {
	req := PySpyCheckRequest{
		PodUID:      "pod-123",
		ContainerID: "container-abc",
	}

	jsonBytes, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded PySpyCheckRequest
	err = json.Unmarshal(jsonBytes, &decoded)
	require.NoError(t, err)

	assert.Equal(t, req.PodUID, decoded.PodUID)
	assert.Equal(t, req.ContainerID, decoded.ContainerID)
}

func TestPySpyCheckResponse_JSON(t *testing.T) {
	resp := PySpyCheckResponse{
		Supported:       true,
		PythonProcesses: []int{100, 200},
		Capabilities:    []string{"CAP_SYS_PTRACE", "CAP_NET_RAW"},
	}

	jsonBytes, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"supported":true`)
	assert.Contains(t, string(jsonBytes), `"python_processes":[100,200]`)
}

func TestPySpyExecuteRequest_JSON(t *testing.T) {
	req := PySpyExecuteRequest{
		TaskID:       "task-123",
		PodUID:       "pod-uid",
		HostPID:      1234,
		ContainerPID: 100,
		Duration:     30,
		Rate:         100,
		Format:       "flamegraph",
		Native:       false,
		SubProcesses: true,
	}

	jsonBytes, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded PySpyExecuteRequest
	err = json.Unmarshal(jsonBytes, &decoded)
	require.NoError(t, err)

	assert.Equal(t, req.TaskID, decoded.TaskID)
	assert.Equal(t, req.HostPID, decoded.HostPID)
	assert.Equal(t, req.Duration, decoded.Duration)
	assert.Equal(t, req.SubProcesses, decoded.SubProcesses)
}

func TestPySpyExecuteResponse_JSON(t *testing.T) {
	resp := PySpyExecuteResponse{
		Success:    true,
		OutputFile: "/var/lib/lens/pyspy/profile.svg",
		FileSize:   12345,
	}

	jsonBytes, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"success":true`)
	assert.Contains(t, string(jsonBytes), `"output_file":"/var/lib/lens/pyspy/profile.svg"`)
	assert.Contains(t, string(jsonBytes), `"file_size":12345`)
}

func TestPySpyExecuteResponse_Error(t *testing.T) {
	resp := PySpyExecuteResponse{
		Success: false,
		Error:   "process not found",
	}

	jsonBytes, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"success":false`)
	assert.Contains(t, string(jsonBytes), `"error":"process not found"`)
}

func TestPySpyFile_Fields(t *testing.T) {
	file := PySpyFile{
		TaskID:       "task-123",
		FileName:     "profile.svg",
		FilePath:     "/var/lib/lens/pyspy/profiles/task-123/profile.svg",
		FileSize:     54321,
		Format:       "flamegraph",
		PodUID:       "pod-uid",
		PodName:      "test-pod",
		PodNamespace: "default",
		NodeName:     "node-1",
		PID:          12345,
		CreatedAt:    time.Now(),
	}

	assert.Equal(t, "task-123", file.TaskID)
	assert.Equal(t, "profile.svg", file.FileName)
	assert.Equal(t, int64(54321), file.FileSize)
	assert.Equal(t, "flamegraph", file.Format)
}

func TestPySpyTask_JSON(t *testing.T) {
	now := time.Now()
	task := PySpyTask{
		TaskID:       "pyspy-abc123",
		Status:       "completed",
		PodUID:       "pod-uid",
		PodName:      "training-job-0",
		PodNamespace: "ml-training",
		NodeName:     "gpu-node-1",
		PID:          9999,
		Duration:     60,
		Format:       "speedscope",
		OutputFile:   "/var/lib/lens/pyspy/profile.json",
		FileSize:     100000,
		CreatedAt:    now,
		StartedAt:    now.Add(time.Second),
		CompletedAt:  now.Add(61 * time.Second),
	}

	jsonBytes, err := json.Marshal(task)
	require.NoError(t, err)

	var decoded PySpyTask
	err = json.Unmarshal(jsonBytes, &decoded)
	require.NoError(t, err)

	assert.Equal(t, task.TaskID, decoded.TaskID)
	assert.Equal(t, task.Status, decoded.Status)
	assert.Equal(t, task.PodName, decoded.PodName)
	assert.Equal(t, task.Duration, decoded.Duration)
	assert.Equal(t, task.Format, decoded.Format)
	assert.Equal(t, task.FileSize, decoded.FileSize)
}

func TestPySpyTaskListRequest_Fields(t *testing.T) {
	req := PySpyTaskListRequest{
		PodUID:       "pod-123",
		PodNamespace: "default",
		NodeName:     "node-1",
		Status:       "completed",
		Limit:        50,
		Offset:       100,
	}

	assert.Equal(t, "pod-123", req.PodUID)
	assert.Equal(t, "default", req.PodNamespace)
	assert.Equal(t, "node-1", req.NodeName)
	assert.Equal(t, "completed", req.Status)
	assert.Equal(t, 50, req.Limit)
	assert.Equal(t, 100, req.Offset)
}

func TestPySpyFileListRequest_Fields(t *testing.T) {
	req := PySpyFileListRequest{
		PodUID:   "pod-123",
		TaskID:   "task-456",
		NodeName: "node-1",
		Limit:    25,
		Offset:   0,
	}

	assert.Equal(t, "pod-123", req.PodUID)
	assert.Equal(t, "task-456", req.TaskID)
	assert.Equal(t, "node-1", req.NodeName)
	assert.Equal(t, 25, req.Limit)
	assert.Equal(t, 0, req.Offset)
}

func TestPySpySampleResponse_Fields(t *testing.T) {
	now := time.Now()
	resp := PySpySampleResponse{
		TaskID:      "pyspy-12345678",
		Status:      "pending",
		NodeName:    "gpu-node-2",
		CreatedAt:   now,
	}

	assert.Equal(t, "pyspy-12345678", resp.TaskID)
	assert.Equal(t, "pending", resp.Status)
	assert.Equal(t, "gpu-node-2", resp.NodeName)
	assert.Equal(t, now, resp.CreatedAt)
	assert.Empty(t, resp.FilePath)
	assert.Zero(t, resp.FileSize)
	assert.True(t, resp.CompletedAt.IsZero())
}

func BenchmarkPySpySampleRequest_SetDefaults(b *testing.B) {
	for i := 0; i < b.N; i++ {
		req := PySpySampleRequest{}
		req.SetDefaults()
	}
}
