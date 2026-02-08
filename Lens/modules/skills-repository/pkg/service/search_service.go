// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/embedding"
)

// SearchService handles search operations for tools
type SearchService struct {
	facade         *database.ToolFacade
	embedding      *embedding.Service
	scoreThreshold float64
}

// NewSearchService creates a new SearchService
func NewSearchService(
	facade *database.ToolFacade,
	embeddingSvc *embedding.Service,
	scoreThreshold float64,
) *SearchService {
	return &SearchService{
		facade:         facade,
		embedding:      embeddingSvc,
		scoreThreshold: scoreThreshold,
	}
}

// SearchResult represents the result of a search
type SearchResult struct {
	Tools interface{} // []database.ToolWithScore or []model.Tool
	Total int
	Mode  string
}

// Search searches tools by query with different modes (keyword, semantic, hybrid)
func (s *SearchService) Search(ctx context.Context, query, toolType, mode string, limit int) (*SearchResult, error) {
	switch mode {
	case "semantic":
		if s.embedding == nil || !s.embedding.IsEnabled() {
			return nil, fmt.Errorf("%w: semantic search is not enabled", ErrNotConfigured)
		}
		emb, err := s.embedding.Generate(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embedding: %w", err)
		}
		results, err := s.facade.SemanticSearch(emb, toolType, limit, s.scoreThreshold)
		if err != nil {
			return nil, err
		}
		return &SearchResult{Tools: results, Total: len(results), Mode: "semantic"}, nil

	case "hybrid":
		if s.embedding == nil || !s.embedding.IsEnabled() {
			// Fallback to keyword search
			tools, err := s.facade.Search(query, toolType, limit)
			if err != nil {
				return nil, err
			}
			return &SearchResult{Tools: tools, Total: len(tools), Mode: "keyword"}, nil
		}
		emb, err := s.embedding.Generate(ctx, query)
		if err != nil {
			// Fallback to keyword search
			tools, err := s.facade.Search(query, toolType, limit)
			if err != nil {
				return nil, err
			}
			return &SearchResult{Tools: tools, Total: len(tools), Mode: "keyword"}, nil
		}
		results, err := s.facade.HybridSearch(query, emb, toolType, limit, s.scoreThreshold)
		if err != nil {
			return nil, err
		}
		return &SearchResult{Tools: results, Total: len(results), Mode: "hybrid"}, nil

	default: // keyword
		tools, err := s.facade.Search(query, toolType, limit)
		if err != nil {
			return nil, err
		}
		return &SearchResult{Tools: tools, Total: len(tools), Mode: "keyword"}, nil
	}
}
