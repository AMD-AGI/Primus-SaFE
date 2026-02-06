// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Service provides embedding generation functionality
type Service struct {
	enabled   bool
	baseURL   string
	apiKey    string
	model     string
	dimension int
	client    *http.Client
}

// Config contains configuration for the embedding service
type Config struct {
	Enabled   bool
	BaseURL   string
	APIKey    string
	Model     string
	Dimension int
}

// NewService creates a new embedding service
func NewService(cfg Config) *Service {
	return &Service{
		enabled:   cfg.Enabled,
		baseURL:   cfg.BaseURL,
		apiKey:    cfg.APIKey,
		model:     cfg.Model,
		dimension: cfg.Dimension,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsEnabled returns whether the embedding service is enabled
func (s *Service) IsEnabled() bool {
	return s.enabled
}

// GetDimension returns the embedding dimension
func (s *Service) GetDimension() int {
	return s.dimension
}

// embeddingRequest is the request body for OpenAI-compatible embedding API
type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// embeddingResponse is the response from OpenAI-compatible embedding API
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Generate generates an embedding vector for the given text
func (s *Service) Generate(ctx context.Context, text string) ([]float32, error) {
	if !s.enabled {
		return nil, fmt.Errorf("embedding service is not enabled")
	}

	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	reqBody := embeddingRequest{
		Model: s.model,
		Input: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/embeddings", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(body, &embResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if embResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", embResp.Error.Message)
	}

	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return embResp.Data[0].Embedding, nil
}

// GenerateForTool generates an embedding for a tool's name and description
func (s *Service) GenerateForTool(ctx context.Context, name, description string) ([]float32, error) {
	text := fmt.Sprintf("%s: %s", name, description)
	return s.Generate(ctx, text)
}
