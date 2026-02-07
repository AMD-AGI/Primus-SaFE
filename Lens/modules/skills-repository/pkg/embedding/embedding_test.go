// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewService(t *testing.T) {
	cfg := Config{
		Enabled:   true,
		BaseURL:   "http://localhost:8000",
		APIKey:    "test-key",
		Model:     "test-model",
		Dimension: 1024,
	}

	svc := NewService(cfg)

	if svc == nil {
		t.Fatal("NewService() returned nil")
	}
	if !svc.enabled {
		t.Error("Service should be enabled")
	}
	if svc.baseURL != cfg.BaseURL {
		t.Errorf("baseURL = %v, want %v", svc.baseURL, cfg.BaseURL)
	}
	if svc.model != cfg.Model {
		t.Errorf("model = %v, want %v", svc.model, cfg.Model)
	}
	if svc.dimension != cfg.Dimension {
		t.Errorf("dimension = %v, want %v", svc.dimension, cfg.Dimension)
	}
}

func TestService_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled service", true},
		{"disabled service", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(Config{Enabled: tt.enabled})
			if svc.IsEnabled() != tt.enabled {
				t.Errorf("IsEnabled() = %v, want %v", svc.IsEnabled(), tt.enabled)
			}
		})
	}
}

func TestService_GetDimension(t *testing.T) {
	svc := NewService(Config{Dimension: 768})
	if svc.GetDimension() != 768 {
		t.Errorf("GetDimension() = %v, want 768", svc.GetDimension())
	}
}

func TestService_Generate_Disabled(t *testing.T) {
	svc := NewService(Config{Enabled: false})
	_, err := svc.Generate(context.Background(), "test text")
	if err == nil {
		t.Error("Generate() should error when service is disabled")
	}
}

func TestService_Generate_EmptyText(t *testing.T) {
	svc := NewService(Config{Enabled: true})
	_, err := svc.Generate(context.Background(), "")
	if err == nil {
		t.Error("Generate() should error for empty text")
	}
}

func TestService_Generate_Success(t *testing.T) {
	// Create mock server
	expectedEmbedding := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/embeddings" {
			t.Errorf("Expected /embeddings, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Expected Authorization header with Bearer token")
		}

		// Parse request body
		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
		}
		if req.Model != "test-model" {
			t.Errorf("Expected model test-model, got %s", req.Model)
		}
		if req.Input != "test text" {
			t.Errorf("Expected input 'test text', got %s", req.Input)
		}

		// Return response
		resp := embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: expectedEmbedding, Index: 0},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := NewService(Config{
		Enabled: true,
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	})

	embedding, err := svc.Generate(context.Background(), "test text")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(embedding) != len(expectedEmbedding) {
		t.Errorf("Embedding length = %d, want %d", len(embedding), len(expectedEmbedding))
	}

	for i, v := range expectedEmbedding {
		if embedding[i] != v {
			t.Errorf("Embedding[%d] = %v, want %v", i, embedding[i], v)
		}
	}
}

func TestService_Generate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{
			Error: &struct {
				Message string `json:"message"`
			}{
				Message: "API error occurred",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := NewService(Config{
		Enabled: true,
		BaseURL: server.URL,
	})

	_, err := svc.Generate(context.Background(), "test text")
	if err == nil {
		t.Error("Generate() should error when API returns error")
	}
}

func TestService_Generate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	svc := NewService(Config{
		Enabled: true,
		BaseURL: server.URL,
	})

	_, err := svc.Generate(context.Background(), "test text")
	if err == nil {
		t.Error("Generate() should error on HTTP error")
	}
}

func TestService_Generate_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := NewService(Config{
		Enabled: true,
		BaseURL: server.URL,
	})

	_, err := svc.Generate(context.Background(), "test text")
	if err == nil {
		t.Error("Generate() should error when no embedding returned")
	}
}

func TestService_GenerateForTool(t *testing.T) {
	expectedEmbedding := []float32{0.1, 0.2, 0.3}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Verify the input is formatted as "name: description"
		expectedInput := "test-tool: A test tool description"
		if req.Input != expectedInput {
			t.Errorf("Input = %q, want %q", req.Input, expectedInput)
		}

		resp := embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: expectedEmbedding, Index: 0},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := NewService(Config{
		Enabled: true,
		BaseURL: server.URL,
	})

	embedding, err := svc.GenerateForTool(context.Background(), "test-tool", "A test tool description")
	if err != nil {
		t.Fatalf("GenerateForTool() error = %v", err)
	}

	if len(embedding) != len(expectedEmbedding) {
		t.Errorf("Embedding length = %d, want %d", len(embedding), len(expectedEmbedding))
	}
}
