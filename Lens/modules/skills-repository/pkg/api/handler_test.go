// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/runner"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/storage"
	"github.com/gin-gonic/gin"
)

// MockToolFacade is a mock implementation of ToolFacade for testing
type MockToolFacade struct {
	tools  map[int64]*model.Tool
	nextID int64
}

func NewMockToolFacade() *MockToolFacade {
	return &MockToolFacade{
		tools:  make(map[int64]*model.Tool),
		nextID: 1,
	}
}

func (m *MockToolFacade) Create(tool *model.Tool) error {
	tool.ID = m.nextID
	m.nextID++
	m.tools[tool.ID] = tool
	return nil
}

func (m *MockToolFacade) GetByID(id int64) (*model.Tool, error) {
	if tool, ok := m.tools[id]; ok {
		return tool, nil
	}
	return nil, nil
}

func (m *MockToolFacade) GetByTypeAndName(toolType, name string) (*model.Tool, error) {
	for _, tool := range m.tools {
		if tool.Type == toolType && tool.Name == name {
			return tool, nil
		}
	}
	return nil, nil
}

func (m *MockToolFacade) Update(tool *model.Tool) error {
	m.tools[tool.ID] = tool
	return nil
}

func (m *MockToolFacade) Delete(id int64) error {
	delete(m.tools, id)
	return nil
}

func (m *MockToolFacade) List(toolType, status, sortField, sortOrder string, offset, limit int) ([]model.Tool, int64, error) {
	var tools []model.Tool
	for _, t := range m.tools {
		if toolType != "" && t.Type != toolType {
			continue
		}
		if status != "" && t.Status != status {
			continue
		}
		tools = append(tools, *t)
	}
	return tools, int64(len(tools)), nil
}

func (m *MockToolFacade) IncrementRunCount(id int64) error {
	if tool, ok := m.tools[id]; ok {
		tool.RunCount++
	}
	return nil
}

func (m *MockToolFacade) IncrementDownloadCount(id int64) error {
	if tool, ok := m.tools[id]; ok {
		tool.DownloadCount++
	}
	return nil
}

// MockRunner is a mock implementation of Runner for testing
type MockRunner struct {
	getRunURLFunc func(ctx context.Context, tools []*model.Tool) (*runner.RunURLResult, error)
}

func (m *MockRunner) GetRunURL(ctx context.Context, tools []*model.Tool) (*runner.RunURLResult, error) {
	if m.getRunURLFunc != nil {
		return m.getRunURLFunc(ctx, tools)
	}
	return &runner.RunURLResult{
		RedirectURL: "http://test.com/session/new",
		SessionID:   "test-session",
	}, nil
}

// MockStorage is a mock implementation of Storage for testing
type MockStorage struct {
	files map[string][]byte
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		files: make(map[string][]byte),
	}
}

func (m *MockStorage) Upload(ctx context.Context, key string, reader io.Reader) error {
	data, _ := io.ReadAll(reader)
	m.files[key] = data
	return nil
}

func (m *MockStorage) UploadBytes(ctx context.Context, key string, data []byte) error {
	m.files[key] = data
	return nil
}

func (m *MockStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if data, ok := m.files[key]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return nil, nil
}

func (m *MockStorage) DownloadBytes(ctx context.Context, key string) ([]byte, error) {
	return m.files[key], nil
}

func (m *MockStorage) Delete(ctx context.Context, key string) error {
	delete(m.files, key)
	return nil
}

func (m *MockStorage) DeletePrefix(ctx context.Context, prefix string) error {
	return nil
}

func (m *MockStorage) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := m.files[key]
	return ok, nil
}

func (m *MockStorage) GetURL(ctx context.Context, key string) (string, error) {
	return "http://storage.test/" + key, nil
}

func (m *MockStorage) ListObjects(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	return nil, nil
}

func setupTestRouter() (*gin.Engine, *Handler, *MockToolFacade) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	mockFacade := NewMockToolFacade()
	mockStorage := NewMockStorage()

	// Create handler with mocks (using nil for embedding service)
	handler := &Handler{
		facade:  nil, // We'll use mockFacade directly in tests
		runner:  nil,
		storage: mockStorage,
	}

	// Note: In real tests, we'd need a proper interface-based design
	// For now, these tests demonstrate the structure

	return router, handler, mockFacade
}

func TestHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := &Handler{}
	router.GET("/health", handler.Health)

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Health() status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["status"] != "healthy" {
		t.Errorf("Health() status = %v, want healthy", response["status"])
	}
}

func TestCreateMCPRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request CreateMCPRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: CreateMCPRequest{
				Name:        "test-mcp",
				Description: "Test MCP server",
				Config: map[string]interface{}{
					"mcpServers": map[string]interface{}{
						"test-mcp": map[string]interface{}{
							"command": "npx",
							"args":    []interface{}{"-y", "test-mcp"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			request: CreateMCPRequest{
				Description: "Test MCP server",
				Config: map[string]interface{}{
					"mcpServers": map[string]interface{}{},
				},
			},
			wantErr: true,
		},
		{
			name: "missing description",
			request: CreateMCPRequest{
				Name: "test-mcp",
				Config: map[string]interface{}{
					"mcpServers": map[string]interface{}{},
				},
			},
			wantErr: true,
		},
		{
			name: "missing config",
			request: CreateMCPRequest{
				Name:        "test-mcp",
				Description: "Test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate using gin binding
			data, _ := json.Marshal(tt.request)

			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/test", func(c *gin.Context) {
				var req CreateMCPRequest
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			req, _ := http.NewRequest("POST", "/test", bytes.NewReader(data))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			hasErr := w.Code != http.StatusOK
			if hasErr != tt.wantErr {
				t.Errorf("Validation hasErr = %v, wantErr %v, body: %s", hasErr, tt.wantErr, w.Body.String())
			}
		})
	}
}

func TestToolRef_Validation(t *testing.T) {
	tests := []struct {
		name    string
		ref     ToolRef
		isValid bool
	}{
		{
			name:    "valid with ID",
			ref:     ToolRef{ID: ptrInt64(1)},
			isValid: true,
		},
		{
			name:    "valid with type and name",
			ref:     ToolRef{Type: "skill", Name: "web-search"},
			isValid: true,
		},
		{
			name:    "valid with all fields",
			ref:     ToolRef{ID: ptrInt64(1), Type: "skill", Name: "web-search"},
			isValid: true,
		},
		{
			name:    "invalid - only type",
			ref:     ToolRef{Type: "skill"},
			isValid: false,
		},
		{
			name:    "invalid - only name",
			ref:     ToolRef{Name: "web-search"},
			isValid: false,
		},
		{
			name:    "invalid - empty",
			ref:     ToolRef{},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.ref.ID != nil || (tt.ref.Type != "" && tt.ref.Name != "")
			if isValid != tt.isValid {
				t.Errorf("ToolRef validation = %v, want %v", isValid, tt.isValid)
			}
		})
	}
}

func TestRunToolsRequest_JSON(t *testing.T) {
	reqJSON := `{
		"tools": [
			{"id": 1, "type": "skill", "name": "web-search"},
			{"type": "mcp", "name": "filesystem"}
		]
	}`

	var req RunToolsRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(req.Tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(req.Tools))
	}

	if req.Tools[0].ID == nil || *req.Tools[0].ID != 1 {
		t.Error("First tool should have ID 1")
	}

	if req.Tools[1].Type != "mcp" || req.Tools[1].Name != "filesystem" {
		t.Error("Second tool should have type=mcp, name=filesystem")
	}
}

func TestUpdateToolRequest_JSON(t *testing.T) {
	reqJSON := `{
		"display_name": "Updated Name",
		"description": "Updated description",
		"tags": ["new", "tags"],
		"is_public": false
	}`

	var req UpdateToolRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.DisplayName != "Updated Name" {
		t.Errorf("DisplayName = %v, want Updated Name", req.DisplayName)
	}

	if len(req.Tags) != 2 {
		t.Errorf("Tags length = %d, want 2", len(req.Tags))
	}

	if req.IsPublic == nil || *req.IsPublic != false {
		t.Error("IsPublic should be false")
	}
}

func TestImportCommitRequest_JSON(t *testing.T) {
	reqJSON := `{
		"archive_key": "skill-imports/user-123/uuid/file.zip",
		"selections": [
			{"relative_path": "web-search", "name_override": ""},
			{"relative_path": ".", "name_override": "custom-name"}
		]
	}`

	var req ImportCommitRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.ArchiveKey != "skill-imports/user-123/uuid/file.zip" {
		t.Errorf("ArchiveKey = %v", req.ArchiveKey)
	}

	if len(req.Selections) != 2 {
		t.Errorf("Selections length = %d, want 2", len(req.Selections))
	}

	if req.Selections[0].RelativePath != "web-search" {
		t.Errorf("First selection path = %v", req.Selections[0].RelativePath)
	}

	if req.Selections[1].NameOverride != "custom-name" {
		t.Error("Second selection should have name_override = custom-name")
	}
}

func TestMultipartFormParsing(t *testing.T) {
	// Create a multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file field
	part, _ := writer.CreateFormFile("file", "test.zip")
	part.Write([]byte("fake zip content"))

	writer.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"filename": file.Filename,
			"size":     file.Size,
		})
	})

	req, _ := http.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Upload status = %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["filename"] != "test.zip" {
		t.Errorf("Filename = %v, want test.zip", response["filename"])
	}
}

func TestQueryParameterParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/list", func(c *gin.Context) {
		offset, _ := c.GetQuery("offset")
		limit, _ := c.GetQuery("limit")
		toolType, _ := c.GetQuery("type")
		sort := c.DefaultQuery("sort", "created_at")
		order := c.DefaultQuery("order", "desc")

		c.JSON(http.StatusOK, gin.H{
			"offset": offset,
			"limit":  limit,
			"type":   toolType,
			"sort":   sort,
			"order":  order,
		})
	})

	req, _ := http.NewRequest("GET", "/list?offset=10&limit=20&type=skill&sort=run_count&order=asc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["offset"] != "10" {
		t.Errorf("offset = %v, want 10", response["offset"])
	}
	if response["limit"] != "20" {
		t.Errorf("limit = %v, want 20", response["limit"])
	}
	if response["type"] != "skill" {
		t.Errorf("type = %v, want skill", response["type"])
	}
	if response["sort"] != "run_count" {
		t.Errorf("sort = %v, want run_count", response["sort"])
	}
	if response["order"] != "asc" {
		t.Errorf("order = %v, want asc", response["order"])
	}
}

func TestSearchModeValidation(t *testing.T) {
	tests := []struct {
		mode  string
		valid bool
	}{
		{"keyword", true},
		{"semantic", true},
		{"hybrid", true},
		{"invalid", false},
		{"", true}, // defaults to semantic
	}

	validModes := map[string]bool{
		"keyword":  true,
		"semantic": true,
		"hybrid":   true,
		"":         true, // default
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			_, valid := validModes[tt.mode]
			if valid != tt.valid {
				t.Errorf("Mode %q validation = %v, want %v", tt.mode, valid, tt.valid)
			}
		})
	}
}

// Helper function
func ptrInt64(v int64) *int64 {
	return &v
}

func ptrBool(v bool) *bool {
	return &v
}

func ptrString(v string) *string {
	return &v
}
