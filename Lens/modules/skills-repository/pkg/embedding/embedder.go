// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package embedding

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/config"
	openai "github.com/sashabaranov/go-openai"
)

// Embedder interface for text embedding generation
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	ModelName() string
}

// OpenAIEmbedder implements Embedder using OpenAI API
type OpenAIEmbedder struct {
	client *openai.Client
	model  string
}

// NewOpenAIEmbedder creates a new OpenAI embedder
func NewOpenAIEmbedder(cfg config.EmbeddingConfig) *OpenAIEmbedder {
	if cfg.APIKey == "" {
		return nil
	}

	clientCfg := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		clientCfg.BaseURL = cfg.BaseURL
	}

	return &OpenAIEmbedder{
		client: openai.NewClientWithConfig(clientCfg),
		model:  cfg.Model,
	}
}

// Embed generates embedding for text
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := e.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.EmbeddingModel(e.model),
	})
	if err != nil {
		return nil, err
	}

	return resp.Data[0].Embedding, nil
}

// ModelName returns the model name
func (e *OpenAIEmbedder) ModelName() string {
	return e.model
}

// NullEmbedder is a no-op embedder for when embedding is disabled
type NullEmbedder struct{}

// Embed returns nil for NullEmbedder
func (e *NullEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

// ModelName returns empty string for NullEmbedder
func (e *NullEmbedder) ModelName() string {
	return ""
}
