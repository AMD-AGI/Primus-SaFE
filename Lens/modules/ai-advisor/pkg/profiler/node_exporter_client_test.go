package profiler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNodeExporterClient(t *testing.T) {
	client := NewNodeExporterClient("http://node-exporter:8080")

	assert.NotNil(t, client)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, "http://node-exporter:8080", client.baseURL)
	assert.Equal(t, 5*time.Minute, client.httpClient.Timeout)
}

func TestNodeExporterClient_ReadProfilerFile_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/container-fs/read", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse request
		var req ReadFileRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "pod-123", req.PodUID)
		assert.Equal(t, "/workspace/logs/profiler.json", req.Path)

		// Return mock response
		testContent := []byte("test profiler content")
		encoded := base64.StdEncoding.EncodeToString(testContent)

		resp := ReadFileResponse{
			Code:    0,
			Message: "success",
			Data: &ReadFileData{
				Content: encoded,
				FileInfo: &FileInfo{
					Path: "/workspace/logs/profiler.json",
					Size: int64(len(testContent)),
				},
				BytesRead: int64(len(testContent)),
				EOF:       true,
				IsBinary:  true,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Test
	client := NewNodeExporterClient(server.URL)
	content, err := client.ReadProfilerFile(context.Background(), "pod-123", "/workspace/logs/profiler.json")

	require.NoError(t, err)
	assert.Equal(t, []byte("test profiler content"), content)
}

func TestNodeExporterClient_ReadProfilerFile_ErrorResponse(t *testing.T) {
	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ReadFileResponse{
			Code:    1,
			Message: "file not found",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)
	_, err := client.ReadProfilerFile(context.Background(), "pod-123", "/invalid/path")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node-exporter error")
}

func TestNodeExporterClient_ReadProfilerFile_HTTPError(t *testing.T) {
	// Create mock server that returns HTTP error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)
	_, err := client.ReadProfilerFile(context.Background(), "pod-123", "/workspace/logs/profiler.json")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node-exporter returned status 500")
}

func TestNodeExporterClient_ReadProfilerFile_InvalidBase64(t *testing.T) {
	// Create mock server that returns invalid base64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ReadFileResponse{
			Code:    0,
			Message: "success",
			Data: &ReadFileData{
				Content:   "invalid-base64!!!",
				BytesRead: 100,
				EOF:       true,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)
	_, err := client.ReadProfilerFile(context.Background(), "pod-123", "/workspace/logs/profiler.json")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode base64")
}

func TestNodeExporterClient_ReadProfilerFileChunked_Success(t *testing.T) {
	// Create test data (30 bytes, will be read in 3 chunks of 10 bytes)
	testData := []byte("0123456789abcdefghijklmnopqrst")
	chunkSize := int64(10)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ReadFileRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Determine which chunk to return
		offset := req.Offset
		length := req.Length

		end := offset + length
		if end > int64(len(testData)) {
			end = int64(len(testData))
		}

		chunk := testData[offset:end]
		encoded := base64.StdEncoding.EncodeToString(chunk)
		eof := end >= int64(len(testData))

		resp := ReadFileResponse{
			Code:    0,
			Message: "success",
			Data: &ReadFileData{
				Content:   encoded,
				BytesRead: int64(len(chunk)),
				EOF:       eof,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)

		callCount++
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)
	content, err := client.ReadProfilerFileChunked(context.Background(), "pod-123", "/workspace/logs/large.json", chunkSize)

	require.NoError(t, err)
	assert.Equal(t, testData, content)
	assert.Equal(t, 3, callCount, "Should make 3 requests for 30 bytes with 10-byte chunks")
}

func TestNodeExporterClient_ReadProfilerFileChunked_ErrorInMiddle(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		
		if callCount == 2 {
			// Return error on second chunk
			resp := ReadFileResponse{
				Code:    1,
				Message: "read error",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Return success for first chunk
		chunk := []byte("chunk data")
		encoded := base64.StdEncoding.EncodeToString(chunk)

		resp := ReadFileResponse{
			Code:    0,
			Message: "success",
			Data: &ReadFileData{
				Content:   encoded,
				BytesRead: int64(len(chunk)),
				EOF:       false,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)
	_, err := client.ReadProfilerFileChunked(context.Background(), "pod-123", "/workspace/logs/large.json", 10)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node-exporter error")
}

func TestNodeExporterClient_GetFileInfo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ReadFileRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, int64(0), req.Length, "Should request 0 length for info only")

		resp := ReadFileResponse{
			Code:    0,
			Message: "success",
			Data: &ReadFileData{
				Content: "",
				FileInfo: &FileInfo{
					Path:    "/workspace/logs/profiler.json",
					Size:    1024000,
					ModTime: time.Now(),
					IsDir:   false,
				},
				BytesRead: 0,
				EOF:       true,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)
	fileInfo, err := client.GetFileInfo(context.Background(), "pod-123", "/workspace/logs/profiler.json")

	require.NoError(t, err)
	assert.NotNil(t, fileInfo)
	assert.Equal(t, "/workspace/logs/profiler.json", fileInfo.Path)
	assert.Equal(t, int64(1024000), fileInfo.Size)
	assert.False(t, fileInfo.IsDir)
}

func TestNodeExporterClient_GetFileInfo_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ReadFileResponse{
			Code:    1,
			Message: "file not found",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)
	_, err := client.GetFileInfo(context.Background(), "pod-123", "/invalid/path")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node-exporter error")
}

func TestReadFileRequest_Serialization(t *testing.T) {
	req := &ReadFileRequest{
		PodUID:        "pod-123",
		PodName:       "training-pod-0",
		PodNamespace:  "default",
		ContainerName: "training",
		Path:          "/workspace/logs/profiler.json",
		Offset:        1024,
		Length:        2048,
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded ReadFileRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, req.PodUID, decoded.PodUID)
	assert.Equal(t, req.PodName, decoded.PodName)
	assert.Equal(t, req.Path, decoded.Path)
	assert.Equal(t, req.Offset, decoded.Offset)
	assert.Equal(t, req.Length, decoded.Length)
}

func TestReadFileResponse_Deserialization(t *testing.T) {
	testContent := []byte("test content")
	encoded := base64.StdEncoding.EncodeToString(testContent)

	jsonData := `{
		"code": 0,
		"message": "success",
		"data": {
			"content": "` + encoded + `",
			"file_info": {
				"path": "/test/file.json",
				"size": 12,
				"is_dir": false
			},
			"bytes_read": 12,
			"eof": true,
			"is_binary": true
		}
	}`

	var resp ReadFileResponse
	err := json.Unmarshal([]byte(jsonData), &resp)
	require.NoError(t, err)

	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, "success", resp.Message)
	assert.NotNil(t, resp.Data)
	assert.Equal(t, encoded, resp.Data.Content)
	assert.Equal(t, int64(12), resp.Data.BytesRead)
	assert.True(t, resp.Data.EOF)
}

func TestNodeExporterClient_ContextCancellation(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.ReadProfilerFile(ctx, "pod-123", "/workspace/logs/profiler.json")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestNodeExporterClient_InvalidURL(t *testing.T) {
	client := NewNodeExporterClient("http://invalid-host-that-does-not-exist:9999")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := client.ReadProfilerFile(ctx, "pod-123", "/workspace/logs/profiler.json")

	assert.Error(t, err)
}

func TestFileInfo_Fields(t *testing.T) {
	now := time.Now()
	info := &FileInfo{
		Path:    "/workspace/logs/profiler.json",
		Size:    1024000,
		ModTime: now,
		IsDir:   false,
	}

	assert.Equal(t, "/workspace/logs/profiler.json", info.Path)
	assert.Equal(t, int64(1024000), info.Size)
	assert.Equal(t, now, info.ModTime)
	assert.False(t, info.IsDir)
}

func TestReadFileData_EmptyContent(t *testing.T) {
	data := &ReadFileData{
		Content:   "",
		BytesRead: 0,
		EOF:       true,
		IsBinary:  false,
	}

	assert.Empty(t, data.Content)
	assert.Equal(t, int64(0), data.BytesRead)
	assert.True(t, data.EOF)
}

func TestNodeExporterClient_LargeFile(t *testing.T) {
	// Simulate reading a 10000 byte file in 1000 byte chunks
	fileSize := int64(10000)
	chunkSize := int64(1000)
	expectedChunks := 10

	callCount := 0
	totalBytesRead := int64(0)
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ReadFileRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Calculate how many bytes to return
		remaining := fileSize - totalBytesRead
		bytesToRead := chunkSize
		if remaining < chunkSize {
			bytesToRead = remaining
		}

		// Create mock chunk with actual size
		mockChunk := make([]byte, bytesToRead)
		encoded := base64.StdEncoding.EncodeToString(mockChunk)

		totalBytesRead += bytesToRead
		eof := totalBytesRead >= fileSize

		resp := ReadFileResponse{
			Code:    0,
			Message: "success",
			Data: &ReadFileData{
				Content:   encoded,
				BytesRead: bytesToRead,
				EOF:       eof,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)

		callCount++
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)
	content, err := client.ReadProfilerFileChunked(context.Background(), "pod-123", "/large-file.json", chunkSize)

	require.NoError(t, err)
	assert.NotNil(t, content)
	assert.Equal(t, fileSize, int64(len(content)), "Content size should match file size")
	assert.Equal(t, expectedChunks, callCount, "Should make expected number of chunk requests")
}

func BenchmarkNodeExporterClient_ReadProfilerFile(b *testing.B) {
	testContent := make([]byte, 1024*1024) // 1MB
	encoded := base64.StdEncoding.EncodeToString(testContent)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ReadFileResponse{
			Code:    0,
			Message: "success",
			Data: &ReadFileData{
				Content:   encoded,
				BytesRead: int64(len(testContent)),
				EOF:       true,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.ReadProfilerFile(context.Background(), "pod-123", "/test.json")
	}
}

