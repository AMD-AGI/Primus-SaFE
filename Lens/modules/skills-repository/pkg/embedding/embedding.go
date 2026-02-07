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
	Model string      `json:"model"`
	Input interface{} `json:"input"` // string or []string for batch
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

// GenerateBatch generates embeddings for multiple texts in a single API call
// Falls back to individual generation if batch fails
func (s *Service) GenerateBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if !s.enabled {
		return nil, fmt.Errorf("embedding service is not enabled")
	}

	if len(texts) == 0 {
		return nil, fmt.Errorf("texts cannot be empty")
	}

	// Filter out empty texts and track indices
	var validTexts []string
	validIndices := make([]int, 0, len(texts))
	for i, text := range texts {
		if text != "" {
			validTexts = append(validTexts, text)
			validIndices = append(validIndices, i)
		}
	}

	if len(validTexts) == 0 {
		return nil, fmt.Errorf("all texts are empty")
	}

	// Try batch API call first
	result, err := s.generateBatchInternal(ctx, validTexts, validIndices, len(texts))
	if err != nil {
		// Fallback: generate individually (like SaFE-QA pattern)
		fmt.Printf("⚠️ Batch embedding failed, falling back to single: %v\n", err)
		return s.generateBatchFallback(ctx, texts)
	}

	fmt.Printf("✅ Batch generated %d embeddings in one API call\n", len(validTexts))
	return result, nil
}

// generateBatchInternal performs the actual batch API call
func (s *Service) generateBatchInternal(ctx context.Context, validTexts []string, validIndices []int, totalLen int) ([][]float32, error) {
	reqBody := embeddingRequest{
		Model: s.model,
		Input: validTexts, // Batch input
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

	if len(embResp.Data) != len(validTexts) {
		return nil, fmt.Errorf("unexpected number of embeddings: got %d, expected %d", len(embResp.Data), len(validTexts))
	}

	// Build result array with embeddings in correct positions
	// Use data.Index from API response to handle potential out-of-order responses
	result := make([][]float32, totalLen)
	for _, data := range embResp.Data {
		if data.Index < 0 || data.Index >= len(validIndices) {
			return nil, fmt.Errorf("invalid index %d in API response", data.Index)
		}
		originalIdx := validIndices[data.Index]
		result[originalIdx] = data.Embedding
	}

	return result, nil
}

// generateBatchFallback generates embeddings one by one when batch fails
func (s *Service) generateBatchFallback(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	successCount := 0

	for i, text := range texts {
		if text == "" {
			continue
		}
		emb, err := s.Generate(ctx, text)
		if err != nil {
			fmt.Printf("⚠️ Single embedding failed for text %d: %v\n", i, err)
			continue
		}
		result[i] = emb
		successCount++
	}

	fmt.Printf("✅ Fallback generated %d/%d embeddings individually\n", successCount, len(texts))
	return result, nil
}
