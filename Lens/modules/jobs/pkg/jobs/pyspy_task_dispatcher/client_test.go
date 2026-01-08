package pyspy_task_dispatcher

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// TestExecuteResponse_Fields tests ExecuteResponse struct fields
func TestExecuteResponse_Fields(t *testing.T) {
	resp := ExecuteResponse{
		Meta: struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{
			Code:    200,
			Message: "success",
		},
		Data: struct {
			Success    bool   `json:"success"`
			OutputFile string `json:"output_file,omitempty"`
			FileSize   int64  `json:"file_size,omitempty"`
			Error      string `json:"error,omitempty"`
		}{
			Success:    true,
			OutputFile: "/tmp/pyspy/task-123/output.svg",
			FileSize:   10240,
		},
	}

	if resp.Meta.Code != 200 {
		t.Errorf("Expected Meta.Code 200, got %d", resp.Meta.Code)
	}
	if resp.Meta.Message != "success" {
		t.Errorf("Expected Meta.Message 'success', got %s", resp.Meta.Message)
	}
	if !resp.Data.Success {
		t.Error("Expected Data.Success to be true")
	}
	if resp.Data.OutputFile != "/tmp/pyspy/task-123/output.svg" {
		t.Errorf("Expected OutputFile path, got %s", resp.Data.OutputFile)
	}
	if resp.Data.FileSize != 10240 {
		t.Errorf("Expected FileSize 10240, got %d", resp.Data.FileSize)
	}
}

// TestExecuteResponse_JSON tests ExecuteResponse JSON marshaling
func TestExecuteResponse_JSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		validate func(t *testing.T, resp ExecuteResponse)
	}{
		{
			name: "success response",
			jsonStr: `{
				"meta": {"code": 200, "message": "success"},
				"data": {"success": true, "output_file": "/tmp/output.svg", "file_size": 5000}
			}`,
			validate: func(t *testing.T, resp ExecuteResponse) {
				if !resp.Data.Success {
					t.Error("Expected success to be true")
				}
				if resp.Data.OutputFile != "/tmp/output.svg" {
					t.Errorf("Expected output file, got %s", resp.Data.OutputFile)
				}
			},
		},
		{
			name: "error response",
			jsonStr: `{
				"meta": {"code": 500, "message": "error"},
				"data": {"success": false, "error": "py-spy execution failed"}
			}`,
			validate: func(t *testing.T, resp ExecuteResponse) {
				if resp.Data.Success {
					t.Error("Expected success to be false")
				}
				if resp.Data.Error != "py-spy execution failed" {
					t.Errorf("Expected error message, got %s", resp.Data.Error)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp ExecuteResponse
			if err := json.Unmarshal([]byte(tt.jsonStr), &resp); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}
			tt.validate(t, resp)
		})
	}
}

// TestPySpyFileInfo_Fields tests PySpyFileInfo struct fields
func TestPySpyFileInfo_Fields(t *testing.T) {
	info := PySpyFileInfo{
		TaskID:    "task-123",
		FileName:  "output.svg",
		FilePath:  "/tmp/pyspy/task-123/output.svg",
		FileSize:  10240,
		Format:    "flamegraph",
		CreatedAt: "2024-01-01T12:00:00Z",
	}

	if info.TaskID != "task-123" {
		t.Errorf("Expected TaskID 'task-123', got %s", info.TaskID)
	}
	if info.FileName != "output.svg" {
		t.Errorf("Expected FileName 'output.svg', got %s", info.FileName)
	}
	if info.FilePath != "/tmp/pyspy/task-123/output.svg" {
		t.Errorf("Expected FilePath, got %s", info.FilePath)
	}
	if info.FileSize != 10240 {
		t.Errorf("Expected FileSize 10240, got %d", info.FileSize)
	}
	if info.Format != "flamegraph" {
		t.Errorf("Expected Format 'flamegraph', got %s", info.Format)
	}
	if info.CreatedAt != "2024-01-01T12:00:00Z" {
		t.Errorf("Expected CreatedAt, got %s", info.CreatedAt)
	}
}

// TestPySpyFileInfo_JSON tests PySpyFileInfo JSON marshaling/unmarshaling
func TestPySpyFileInfo_JSON(t *testing.T) {
	original := PySpyFileInfo{
		TaskID:    "task-456",
		FileName:  "profile.speedscope",
		FilePath:  "/data/pyspy/task-456/profile.speedscope",
		FileSize:  20480,
		Format:    "speedscope",
		CreatedAt: "2024-06-15T10:30:00Z",
	}

	// Marshal
	jsonBytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed PySpyFileInfo
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify
	if parsed.TaskID != original.TaskID {
		t.Errorf("TaskID mismatch: got %s, want %s", parsed.TaskID, original.TaskID)
	}
	if parsed.FileName != original.FileName {
		t.Errorf("FileName mismatch: got %s, want %s", parsed.FileName, original.FileName)
	}
	if parsed.FileSize != original.FileSize {
		t.Errorf("FileSize mismatch: got %d, want %d", parsed.FileSize, original.FileSize)
	}
	if parsed.Format != original.Format {
		t.Errorf("Format mismatch: got %s, want %s", parsed.Format, original.Format)
	}
}

// TestNodeExporterClient_Deprecated tests deprecated NodeExporterClient
func TestNodeExporterClient_Deprecated(t *testing.T) {
	client := NewNodeExporterClient()
	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	ctx := context.Background()

	// All deprecated methods should return errors
	t.Run("ExecutePySpy deprecated", func(t *testing.T) {
		_, err := client.ExecutePySpy(ctx, "localhost:8080", nil)
		if err == nil {
			t.Error("Expected error from deprecated method")
		}
	})

	t.Run("DownloadFile deprecated", func(t *testing.T) {
		_, err := client.DownloadFile(ctx, "localhost:8080", "task-1", "file.svg")
		if err == nil {
			t.Error("Expected error from deprecated method")
		}
	})

	t.Run("DeleteFile deprecated", func(t *testing.T) {
		err := client.DeleteFile(ctx, "localhost:8080", "task-1")
		if err == nil {
			t.Error("Expected error from deprecated method")
		}
	})

	t.Run("CheckCompatibility deprecated", func(t *testing.T) {
		_, err := client.CheckCompatibility(ctx, "localhost:8080", "pod-123")
		if err == nil {
			t.Error("Expected error from deprecated method")
		}
	})
}

// TestUnmarshalJSON tests the unmarshalJSON helper
func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name:    "valid JSON object",
			data:    `{"key": "value"}`,
			wantErr: false,
		},
		{
			name:    "valid JSON array",
			data:    `[1, 2, 3]`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			data:    `{invalid}`,
			wantErr: true,
		},
		{
			name:    "empty string",
			data:    ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result interface{}
			err := unmarshalJSON([]byte(tt.data), &result)
			if (err != nil) != tt.wantErr {
				t.Errorf("unmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestPySpyClient_Fields tests PySpyClient struct
func TestPySpyClient_Fields(t *testing.T) {
	// We can't create a real client without K8s, but we can test the struct
	client := &PySpyClient{
		baseURL: "http://localhost:8080",
	}

	if client.baseURL != "http://localhost:8080" {
		t.Errorf("Expected baseURL 'http://localhost:8080', got %s", client.baseURL)
	}
}

// TestAPIEndpoints tests API endpoint constants
func TestAPIEndpoints(t *testing.T) {
	if pyspyExecuteAPI != "/v1/pyspy/execute" {
		t.Errorf("Expected pyspyExecuteAPI '/v1/pyspy/execute', got %s", pyspyExecuteAPI)
	}
	if pyspyCheckAPI != "/v1/pyspy/check" {
		t.Errorf("Expected pyspyCheckAPI '/v1/pyspy/check', got %s", pyspyCheckAPI)
	}
	if pyspyFileAPI != "/v1/pyspy/file" {
		t.Errorf("Expected pyspyFileAPI '/v1/pyspy/file', got %s", pyspyFileAPI)
	}
}

// TestExecuteRequest_Construction tests constructing an execute request from PySpyTaskExt
func TestExecuteRequest_Construction(t *testing.T) {
	ext := &model.PySpyTaskExt{
		TaskID:       "task-789",
		PodUID:       "pod-uid-123",
		HostPID:      12345,
		ContainerPID: 1234,
		Duration:     30,
		Rate:         100,
		Format:       "flamegraph",
		Native:       true,
		SubProcesses: true,
	}

	// Simulate request construction (as done in ExecutePySpy)
	req := &model.PySpyExecuteRequest{
		TaskID:       ext.TaskID,
		PodUID:       ext.PodUID,
		HostPID:      ext.HostPID,
		ContainerPID: ext.ContainerPID,
		Duration:     ext.Duration,
		Rate:         ext.Rate,
		Format:       ext.Format,
		Native:       ext.Native,
		SubProcesses: ext.SubProcesses,
	}

	if req.TaskID != "task-789" {
		t.Errorf("Expected TaskID 'task-789', got %s", req.TaskID)
	}
	if req.PodUID != "pod-uid-123" {
		t.Errorf("Expected PodUID 'pod-uid-123', got %s", req.PodUID)
	}
	if req.HostPID != 12345 {
		t.Errorf("Expected HostPID 12345, got %d", req.HostPID)
	}
	if req.ContainerPID != 1234 {
		t.Errorf("Expected ContainerPID 1234, got %d", req.ContainerPID)
	}
	if req.Duration != 30 {
		t.Errorf("Expected Duration 30, got %d", req.Duration)
	}
	if req.Rate != 100 {
		t.Errorf("Expected Rate 100, got %d", req.Rate)
	}
	if req.Format != "flamegraph" {
		t.Errorf("Expected Format 'flamegraph', got %s", req.Format)
	}
	if !req.Native {
		t.Error("Expected Native to be true")
	}
	if !req.SubProcesses {
		t.Error("Expected SubProcesses to be true")
	}
}
