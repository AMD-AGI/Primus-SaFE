// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockServer creates a test HTTP server with custom handler
func mockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// mockSuccessResponse creates a standard success response
func mockSuccessResponse(data interface{}) map[string]interface{} {
	return map[string]interface{}{
		"code": 0,
		"data": data,
		"msg":  "",
	}
}

// mockErrorResponse creates a standard error response
func mockErrorResponse(code int, msg string) map[string]interface{} {
	return map[string]interface{}{
		"code": code,
		"data": nil,
		"msg":  msg,
	}
}

func TestDefaultConfig(t *testing.T) {
	baseURL := "http://test:8989"
	cfg := DefaultConfig(baseURL)

	assert.Equal(t, baseURL, cfg.BaseURL)
	assert.Equal(t, 60*time.Second, cfg.Timeout)
	assert.Equal(t, 2, cfg.RetryCount)
	assert.Equal(t, 2*time.Second, cfg.RetryWaitTime)
	assert.False(t, cfg.Debug)
}

func TestNewClient(t *testing.T) {
	t.Run("with custom config", func(t *testing.T) {
		cfg := &Config{
			BaseURL:       "http://custom:9999",
			Timeout:       30 * time.Second,
			RetryCount:    3,
			RetryWaitTime: 1 * time.Second,
			Debug:         true,
		}
		client := NewClient(cfg)

		assert.NotNil(t, client)
		assert.Equal(t, cfg.BaseURL, client.BaseURL())
		assert.NotNil(t, client.GetRestyClient())
	})

	t.Run("with nil config", func(t *testing.T) {
		client := NewClient(nil)

		assert.NotNil(t, client)
		assert.Equal(t, "http://primus-lens-node-exporter:8989", client.BaseURL())
	})
}

func TestNewClientForNode(t *testing.T) {
	client := NewClientForNode("worker-node-1")

	assert.NotNil(t, client)
	assert.Equal(t, "http://primus-lens-node-exporter.primus-lens.svc.cluster.local:8989", client.BaseURL())
}

func TestGetRestyClient(t *testing.T) {
	client := NewClient(DefaultConfig("http://test:8989"))
	restyClient := client.GetRestyClient()

	assert.NotNil(t, restyClient)
}

func TestGetPodProcessTree(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expectedData := &types.PodProcessTree{
			PodUID:      "test-pod-uid",
			PodName:     "test-pod",
			PodNamespace: "default",
			Containers: []*types.ContainerProcessTree{
				{
					ContainerID:   "container-123",
					ContainerName: "main",
					RootProcess: &types.ProcessInfo{
						HostPID:  1,
						HostPPID: 0,
						Comm:     "python",
						Cmdline:  "python train.py",
						IsPython: true,
					},
					AllProcesses: []*types.ProcessInfo{
						{
							HostPID:  1,
							HostPPID: 0,
							Comm:     "python",
							Cmdline:  "python train.py",
							IsPython: true,
						},
					},
					ProcessCount: 1,
					PythonCount:  1,
				},
			},
			TotalProcesses: 1,
			TotalPython:    1,
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/process-tree/pod", r.URL.Path)
			assert.Equal(t, "POST", r.Method)

			var req types.ProcessTreeRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ProcessTreeRequest{
			PodUID: "test-pod-uid",
		}

		result, err := client.GetPodProcessTree(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, expectedData.PodUID, result.PodUID)
		assert.Len(t, result.Containers, 1)
		assert.Equal(t, 1, result.TotalProcesses)
	})

	t.Run("api error code", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockErrorResponse(1, "pod not found"))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ProcessTreeRequest{PodUID: "test-pod-uid"}

		result, err := client.GetPodProcessTree(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "returned code 1")
	})

	t.Run("http error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ProcessTreeRequest{PodUID: "test-pod-uid"}

		result, err := client.GetPodProcessTree(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("context timeout", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ProcessTreeRequest{PodUID: "test-pod-uid"}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		result, err := client.GetPodProcessTree(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestFindPythonProcesses(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expectedData := []*types.ProcessInfo{
			{
				HostPID:  100,
				HostPPID: 1,
				Comm:     "python3",
				Cmdline:  "python3 train.py",
				IsPython: true,
			},
			{
				HostPID:  200,
				HostPPID: 1,
				Comm:     "python",
				Cmdline:  "python eval.py",
				IsPython: true,
			},
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/process-tree/python", r.URL.Path)
			assert.Equal(t, "POST", r.Method)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.FindPythonProcesses(context.Background(), "test-pod-uid")

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 2)
		assert.Equal(t, 100, result[0].HostPID)
		assert.Equal(t, 200, result[1].HostPID)
	})

	t.Run("no python processes found", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse([]*types.ProcessInfo{}))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.FindPythonProcesses(context.Background(), "test-pod-uid")

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 0)
	})

	t.Run("api error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockErrorResponse(2, "failed to list processes"))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.FindPythonProcesses(context.Background(), "test-pod-uid")

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("http error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.FindPythonProcesses(context.Background(), "test-pod-uid")

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestFindTensorboardFiles(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expectedData := &types.TensorboardFilesResponse{
			PodUID:       "test-pod-uid",
			PodName:      "test-pod",
			PodNamespace: "default",
			Files: []*types.TensorboardFileInfo{
				{
					PID:      100,
					FD:       "3",
					FilePath: "/logs/events.out.tfevents.123",
					FileName: "events.out.tfevents.123",
				},
			},
			TotalProcesses: 1,
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/process-tree/tensorboard", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.FindTensorboardFiles(context.Background(), "test-pod-uid", "test-pod", "default")

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "test-pod-uid", result.PodUID)
		assert.Len(t, result.Files, 1)
	})
}

func TestGetProcessEnvironment(t *testing.T) {
	t.Run("success with filter", func(t *testing.T) {
		expectedData := &types.ProcessEnvResponse{
			PodUID: "test-pod-uid",
			Processes: []*types.ProcessEnvInfo{
				{
					PID:     100,
					Cmdline: "python train.py",
					Environment: map[string]string{
						"CUDA_VISIBLE_DEVICES": "0,1",
						"NCCL_DEBUG":           "INFO",
					},
				},
			},
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/process-tree/env", r.URL.Path)

			var req struct {
				PodUID       string `json:"pod_uid"`
				PID          int    `json:"pid,omitempty"`
				FilterPrefix string `json:"filter_prefix,omitempty"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "CUDA", req.FilterPrefix)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetProcessEnvironment(context.Background(), "test-pod-uid", 100, "CUDA")

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.Processes, 1)
		assert.Len(t, result.Processes[0].Environment, 2)
	})

	t.Run("success without filter", func(t *testing.T) {
		expectedData := &types.ProcessEnvResponse{
			PodUID:    "test-pod-uid",
			Processes: []*types.ProcessEnvInfo{},
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetProcessEnvironment(context.Background(), "test-pod-uid", 0, "")

		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}

func TestGetProcessArguments(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expectedData := &types.ProcessArgsResponse{
			PodUID: "test-pod-uid",
			Processes: []*types.ProcessArgInfo{
				{
					PID:     100,
					Cmdline: "python train.py --epochs 100",
					Args:    []string{"python", "train.py", "--epochs", "100"},
				},
			},
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/process-tree/args", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetProcessArguments(context.Background(), "test-pod-uid", 100)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.Processes, 1)
		assert.Equal(t, []string{"python", "train.py", "--epochs", "100"}, result.Processes[0].Args)
	})
}

func TestReadContainerFile(t *testing.T) {
	t.Run("success with base64 decode", func(t *testing.T) {
		originalContent := "Hello, World!\nThis is a test file."
		encodedContent := base64.StdEncoding.EncodeToString([]byte(originalContent))

		expectedData := &types.ContainerFileReadResponse{
			Content: encodedContent,
			FileInfo: &types.ContainerFileInfo{
				Path: "/test/file.txt",
				Size: int64(len(originalContent)),
			},
			BytesRead: int64(len(originalContent)),
			EOF:       true,
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/container-fs/read", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ContainerFileReadRequest{
			PodUID:        "test-pod-uid",
			ContainerName: "test-container",
			Path:          "/test/file.txt",
		}

		result, err := client.ReadContainerFile(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, originalContent, result.Content)
		assert.Equal(t, "/test/file.txt", result.FileInfo.Path)
	})

	t.Run("invalid base64", func(t *testing.T) {
		expectedData := &types.ContainerFileReadResponse{
			Content: "invalid-base64!!!",
			FileInfo: &types.ContainerFileInfo{
				Path: "/test/file.txt",
				Size: 100,
			},
			BytesRead: 100,
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ContainerFileReadRequest{
			PodUID: "test-pod-uid",
			Path:   "/test/file.txt",
		}

		result, err := client.ReadContainerFile(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to decode base64")
	})

	t.Run("empty content", func(t *testing.T) {
		expectedData := &types.ContainerFileReadResponse{
			Content: "",
			FileInfo: &types.ContainerFileInfo{
				Path: "/test/empty.txt",
				Size: 0,
			},
			BytesRead: 0,
			EOF:       true,
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ContainerFileReadRequest{
			PodUID: "test-pod-uid",
			Path:   "/test/empty.txt",
		}

		result, err := client.ReadContainerFile(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "", result.Content)
	})

	t.Run("api error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockErrorResponse(11, "file not found"))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ContainerFileReadRequest{
			PodUID: "test-pod-uid",
			Path:   "/nonexistent.txt",
		}

		result, err := client.ReadContainerFile(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("http error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ContainerFileReadRequest{
			PodUID: "test-pod-uid",
			Path:   "/test.txt",
		}

		result, err := client.ReadContainerFile(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestListContainerDirectory(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expectedData := &types.ContainerDirectoryListResponse{
			Files: []*types.ContainerFileInfo{
				{
					Path:  "/workspace/train.py",
					Size:  2048,
					IsDir: false,
				},
				{
					Path:  "/workspace/data",
					IsDir: true,
				},
			},
			Total: 2,
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/container-fs/list", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ContainerDirectoryListRequest{
			PodUID:        "test-pod-uid",
			ContainerName: "test-container",
			Path:          "/workspace",
		}

		result, err := client.ListContainerDirectory(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.Files, 2)
		assert.Equal(t, 2, result.Total)
	})

	t.Run("directory not found", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockErrorResponse(10, "directory not found"))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ContainerDirectoryListRequest{
			PodUID: "test-pod-uid",
			Path:   "/nonexistent",
		}

		result, err := client.ListContainerDirectory(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("http error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		req := &types.ContainerDirectoryListRequest{
			PodUID: "test-pod-uid",
			Path:   "/workspace",
		}

		result, err := client.ListContainerDirectory(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestGetContainerFileInfo(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expectedData := &types.ContainerFileInfo{
			Path:  "/config.yaml",
			Size:  512,
			IsDir: false,
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/container-fs/info", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetContainerFileInfo(context.Background(), "test-pod-uid", "test-pod", "default", "test-container", "/config.yaml")

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "/config.yaml", result.Path)
		assert.Equal(t, int64(512), result.Size)
		assert.False(t, result.IsDir)
	})
}

func TestGetTensorBoardLogs(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expectedData := &types.TensorBoardLogInfo{
			LogDir: "/workspace/logs",
			EventFiles: []*types.ContainerFileInfo{
				{
					Path:  "/workspace/logs/events.out.tfevents.123",
					Size:  4096,
					IsDir: false,
				},
			},
			TotalSize: 4096,
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/container-fs/tensorboard/logs", r.URL.Path)

			var req struct {
				LogDir string `json:"log_dir"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "/workspace/logs", req.LogDir)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetTensorBoardLogs(context.Background(), "test-pod-uid", "test-pod", "default", "test-container", "/workspace/logs")

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "/workspace/logs", result.LogDir)
		assert.Len(t, result.EventFiles, 1)
	})
}

func TestReadTensorBoardEvent(t *testing.T) {
	t.Run("success with offset and length", func(t *testing.T) {
		content := "binary tensorboard event data"
		encodedContent := base64.StdEncoding.EncodeToString([]byte(content))

		expectedData := &types.ContainerFileReadResponse{
			Content: encodedContent,
			FileInfo: &types.ContainerFileInfo{
				Path: "/logs/events.out.tfevents.123",
				Size: int64(len(content)),
			},
			BytesRead: int64(len(content)),
			EOF:       false,
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/container-fs/tensorboard/event", r.URL.Path)

			var req struct {
				EventFile string `json:"event_file"`
				Offset    int64  `json:"offset,omitempty"`
				Length    int64  `json:"length,omitempty"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, int64(100), req.Offset)
			assert.Equal(t, int64(500), req.Length)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.ReadTensorBoardEvent(context.Background(), "test-pod-uid", "test-pod", "default", "test-container", "/logs/events.out.tfevents.123", 100, 500)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "/logs/events.out.tfevents.123", result.FileInfo.Path)
	})

	t.Run("success without offset and length", func(t *testing.T) {
		expectedData := &types.ContainerFileReadResponse{
			Content: "",
			FileInfo: &types.ContainerFileInfo{
				Path: "/logs/events.out.tfevents.123",
				Size: 0,
			},
			BytesRead: 0,
			EOF:       true,
		}

		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				EventFile string `json:"event_file"`
				Offset    int64  `json:"offset,omitempty"`
				Length    int64  `json:"length,omitempty"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, int64(0), req.Offset)
			assert.Equal(t, int64(0), req.Length)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockSuccessResponse(expectedData))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.ReadTensorBoardEvent(context.Background(), "test-pod-uid", "test-pod", "default", "test-container", "/logs/events.out.tfevents.123", 0, 0)

		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("api error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockErrorResponse(20, "file not found"))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.ReadTensorBoardEvent(context.Background(), "test-pod-uid", "test-pod", "default", "test-container", "/logs/nonexistent.tfevents", 0, 0)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "returned code 20")
	})

	t.Run("http error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.ReadTensorBoardEvent(context.Background(), "test-pod-uid", "test-pod", "default", "test-container", "/logs/events.tfevents", 0, 0)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestGetTensorBoardLogsError(t *testing.T) {
	t.Run("api error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockErrorResponse(15, "log directory not accessible"))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetTensorBoardLogs(context.Background(), "test-pod-uid", "test-pod", "default", "test-container", "/nonexistent")

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("http error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetTensorBoardLogs(context.Background(), "test-pod-uid", "test-pod", "default", "test-container", "/logs")

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestGetContainerFileInfoError(t *testing.T) {
	t.Run("api error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockErrorResponse(12, "file not found"))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetContainerFileInfo(context.Background(), "test-pod-uid", "test-pod", "default", "test-container", "/nonexistent.txt")

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("http error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetContainerFileInfo(context.Background(), "test-pod-uid", "test-pod", "default", "test-container", "/test.txt")

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestFindTensorboardFilesError(t *testing.T) {
	t.Run("api error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockErrorResponse(5, "pod not found"))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.FindTensorboardFiles(context.Background(), "nonexistent-pod", "test-pod", "default")

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("http error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.FindTensorboardFiles(context.Background(), "test-pod-uid", "test-pod", "default")

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestGetProcessEnvironmentError(t *testing.T) {
	t.Run("api error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockErrorResponse(7, "process not found"))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetProcessEnvironment(context.Background(), "test-pod-uid", 9999, "")

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("http error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetProcessEnvironment(context.Background(), "test-pod-uid", 100, "")

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestGetProcessArgumentsError(t *testing.T) {
	t.Run("api error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockErrorResponse(8, "process not found"))
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetProcessArguments(context.Background(), "test-pod-uid", 9999)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("http error", func(t *testing.T) {
		server := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		defer server.Close()

		client := NewClient(DefaultConfig(server.URL))
		result, err := client.GetProcessArguments(context.Background(), "test-pod-uid", 100)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// Test error scenarios for all methods
func TestErrorScenarios(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*Client, context.Context) error
	}{
		{
			name: "GetPodProcessTree network error",
			testFunc: func(c *Client, ctx context.Context) error {
				_, err := c.GetPodProcessTree(ctx, &types.ProcessTreeRequest{PodUID: "test"})
				return err
			},
		},
		{
			name: "FindPythonProcesses network error",
			testFunc: func(c *Client, ctx context.Context) error {
				_, err := c.FindPythonProcesses(ctx, "test")
				return err
			},
		},
		{
			name: "FindTensorboardFiles network error",
			testFunc: func(c *Client, ctx context.Context) error {
				_, err := c.FindTensorboardFiles(ctx, "test", "pod", "ns")
				return err
			},
		},
		{
			name: "GetProcessEnvironment network error",
			testFunc: func(c *Client, ctx context.Context) error {
				_, err := c.GetProcessEnvironment(ctx, "test", 100, "")
				return err
			},
		},
		{
			name: "GetProcessArguments network error",
			testFunc: func(c *Client, ctx context.Context) error {
				_, err := c.GetProcessArguments(ctx, "test", 100)
				return err
			},
		},
		{
			name: "ReadContainerFile network error",
			testFunc: func(c *Client, ctx context.Context) error {
				_, err := c.ReadContainerFile(ctx, &types.ContainerFileReadRequest{PodUID: "test", Path: "/test"})
				return err
			},
		},
		{
			name: "ListContainerDirectory network error",
			testFunc: func(c *Client, ctx context.Context) error {
				_, err := c.ListContainerDirectory(ctx, &types.ContainerDirectoryListRequest{PodUID: "test", Path: "/test"})
				return err
			},
		},
		{
			name: "GetContainerFileInfo network error",
			testFunc: func(c *Client, ctx context.Context) error {
				_, err := c.GetContainerFileInfo(ctx, "test", "pod", "ns", "container", "/test")
				return err
			},
		},
		{
			name: "GetTensorBoardLogs network error",
			testFunc: func(c *Client, ctx context.Context) error {
				_, err := c.GetTensorBoardLogs(ctx, "test", "pod", "ns", "container", "/logs")
				return err
			},
		},
		{
			name: "ReadTensorBoardEvent network error",
			testFunc: func(c *Client, ctx context.Context) error {
				_, err := c.ReadTensorBoardEvent(ctx, "test", "pod", "ns", "container", "/event", 0, 0)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client with invalid URL to simulate network error
			client := NewClient(DefaultConfig("http://invalid-host-that-does-not-exist:9999"))
			cfg := DefaultConfig("http://invalid-host-that-does-not-exist:9999")
			cfg.Timeout = 100 * time.Millisecond
			cfg.RetryCount = 0
			client = NewClient(cfg)

			err := tt.testFunc(client, context.Background())
			assert.Error(t, err)
		})
	}
}

