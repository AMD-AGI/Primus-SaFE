// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/embedding"
	"github.com/pgvector/pgvector-go"
)

// ToolsetService handles business logic for toolsets
type ToolsetService struct {
	facade         *database.ToolsetFacade
	toolFacade     *database.ToolFacade
	embedding      *embedding.Service
	scoreThreshold float64
}

// NewToolsetService creates a new ToolsetService
func NewToolsetService(
	facade *database.ToolsetFacade,
	toolFacade *database.ToolFacade,
	embeddingSvc *embedding.Service,
	scoreThreshold float64,
) *ToolsetService {
	return &ToolsetService{
		facade:         facade,
		toolFacade:     toolFacade,
		embedding:      embeddingSvc,
		scoreThreshold: scoreThreshold,
	}
}

// --- Input/Output Types ---

// CreateToolsetInput represents input for creating a toolset
type CreateToolsetInput struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	IconURL     string   `json:"icon_url"`
	IsPublic    *bool    `json:"is_public"`
	ToolIDs     []int64  `json:"tool_ids"` // Optional: add tools on creation
	UserID      string
	Username    string
}

// UpdateToolsetInput represents input for updating a toolset
type UpdateToolsetInput struct {
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	IconURL     string   `json:"icon_url"`
	IsPublic    *bool    `json:"is_public"`
}

// ToolsetListInput represents input for listing toolsets
type ToolsetListInput struct {
	Owner     string
	SortField string
	SortOrder string
	Offset    int
	Limit     int
	UserID    string
	IsAdmin   bool
}

// ToolsetListResult represents the result of listing toolsets
type ToolsetListResult struct {
	Toolsets []model.Toolset `json:"toolsets"`
	Total    int64           `json:"total"`
	Offset   int             `json:"offset"`
	Limit    int             `json:"limit"`
}

// ToolsetDetail represents a toolset with its tools
type ToolsetDetail struct {
	model.Toolset
	Tools []model.Tool `json:"tools"`
}

// ToolsetSearchResult represents search results for toolsets
type ToolsetSearchResult struct {
	Toolsets interface{} `json:"toolsets"` // []model.Toolset or []database.ToolsetWithScore
	Total    int         `json:"total"`
	Mode     string      `json:"mode"`
}

// AddToolsInput represents input for adding tools to a toolset
type AddToolsInput struct {
	ToolIDs []int64 `json:"tool_ids"`
}

// --- Service Methods ---

// Create creates a new toolset
func (s *ToolsetService) Create(ctx context.Context, input CreateToolsetInput) (*model.Toolset, error) {
	displayName := input.DisplayName
	if displayName == "" {
		displayName = input.Name
	}

	isPublic := true
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	toolset := &model.Toolset{
		Name:          input.Name,
		DisplayName:   displayName,
		Description:   input.Description,
		Tags:          input.Tags,
		IconURL:       input.IconURL,
		OwnerUserID:   input.UserID,
		OwnerUserName: input.Username,
		IsPublic:      isPublic,
	}

	// Generate embedding before insert so data and vector are written together
	if emb, err := s.generateEmbedding(ctx, toolset.Name, toolset.Description); err == nil {
		toolset.Embedding = emb
	}

	if err := s.facade.Create(toolset); err != nil {
		return nil, err
	}

	// Add initial tools if provided
	if len(input.ToolIDs) > 0 {
		added, err := s.facade.AddTools(toolset.ID, input.ToolIDs)
		if err != nil {
			return nil, fmt.Errorf("toolset created but failed to add tools: %w", err)
		}
		toolset.ToolCount = added
	}

	return toolset, nil
}

// List retrieves toolsets with filters and sorting
func (s *ToolsetService) List(ctx context.Context, input ToolsetListInput) (*ToolsetListResult, error) {
	// Validate sort field
	validSortFields := map[string]bool{
		"created_at": true, "updated_at": true, "tool_count": true,
	}
	if !validSortFields[input.SortField] {
		input.SortField = "created_at"
	}
	if input.SortOrder != "asc" && input.SortOrder != "desc" {
		input.SortOrder = "desc"
	}

	ownerOnly := input.Owner == "me"
	toolsets, total, err := s.facade.List(input.SortField, input.SortOrder, input.Offset, input.Limit, input.UserID, ownerOnly, input.IsAdmin)
	if err != nil {
		return nil, err
	}

	return &ToolsetListResult{
		Toolsets: toolsets,
		Total:    total,
		Offset:   input.Offset,
		Limit:    input.Limit,
	}, nil
}

// GetToolset retrieves a toolset by ID with its tools.
// Admins can view any toolset; regular users can only view public or their own.
func (s *ToolsetService) GetToolset(ctx context.Context, id int64, userID string, isAdmin bool) (*ToolsetDetail, error) {
	toolset, err := s.facade.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("toolset %w", ErrNotFound)
	}

	if !isAdmin && !toolset.IsPublic && toolset.OwnerUserID != userID {
		return nil, ErrAccessDenied
	}

	// Get tools in this toolset
	tools, err := s.facade.GetTools(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get toolset tools: %w", err)
	}

	return &ToolsetDetail{
		Toolset: *toolset,
		Tools:   tools,
	}, nil
}

// UpdateToolset updates a toolset with access control.
// Admins can update any toolset; regular users can only update their own.
func (s *ToolsetService) UpdateToolset(ctx context.Context, id int64, input UpdateToolsetInput, userID string, isAdmin bool) (*model.Toolset, error) {
	toolset, err := s.facade.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("toolset %w", ErrNotFound)
	}

	if !isAdmin && toolset.OwnerUserID != userID {
		return nil, fmt.Errorf("%w: only owner or admin can update", ErrAccessDenied)
	}

	needReEmbed := false

	if input.DisplayName != "" {
		toolset.DisplayName = input.DisplayName
	}
	if input.Description != "" {
		if toolset.Description != input.Description {
			needReEmbed = true
		}
		toolset.Description = input.Description
	}
	if input.Tags != nil {
		toolset.Tags = input.Tags
	}
	if input.IconURL != "" {
		toolset.IconURL = input.IconURL
	}
	if input.IsPublic != nil {
		toolset.IsPublic = *input.IsPublic
	}

	// Re-generate embedding before update so data and vector are written together
	if needReEmbed {
		if emb, err := s.generateEmbedding(ctx, toolset.Name, toolset.Description); err == nil {
			toolset.Embedding = emb
		}
	}

	if err := s.facade.Update(toolset); err != nil {
		return nil, err
	}

	return toolset, nil
}

// DeleteToolset deletes a toolset with access control.
// Admins can delete any toolset; regular users can only delete their own.
func (s *ToolsetService) DeleteToolset(ctx context.Context, id int64, userID string, isAdmin bool) error {
	toolset, err := s.facade.GetByID(id)
	if err != nil {
		return fmt.Errorf("toolset %w", ErrNotFound)
	}

	if !isAdmin && toolset.OwnerUserID != userID {
		return fmt.Errorf("%w: only owner or admin can delete", ErrAccessDenied)
	}

	return s.facade.Delete(id)
}

// AddTools adds tools to a toolset.
// Admins can modify any toolset; regular users can only modify their own.
func (s *ToolsetService) AddTools(ctx context.Context, toolsetID int64, input AddToolsInput, userID string, isAdmin bool) (int, error) {
	toolset, err := s.facade.GetByID(toolsetID)
	if err != nil {
		return 0, fmt.Errorf("toolset %w", ErrNotFound)
	}

	if !isAdmin && toolset.OwnerUserID != userID {
		return 0, fmt.Errorf("%w: only owner or admin can modify toolset", ErrAccessDenied)
	}

	// Validate that all tool IDs exist
	for _, toolID := range input.ToolIDs {
		if _, err := s.toolFacade.GetByID(toolID); err != nil {
			return 0, fmt.Errorf("tool %d %w", toolID, ErrNotFound)
		}
	}

	return s.facade.AddTools(toolsetID, input.ToolIDs)
}

// RemoveTool removes a tool from a toolset.
// Admins can modify any toolset; regular users can only modify their own.
func (s *ToolsetService) RemoveTool(ctx context.Context, toolsetID, toolID int64, userID string, isAdmin bool) error {
	toolset, err := s.facade.GetByID(toolsetID)
	if err != nil {
		return fmt.Errorf("toolset %w", ErrNotFound)
	}

	if !isAdmin && toolset.OwnerUserID != userID {
		return fmt.Errorf("%w: only owner or admin can modify toolset", ErrAccessDenied)
	}

	return s.facade.RemoveTool(toolsetID, toolID)
}

// Search searches toolsets by query.
// Admins can search all toolsets; regular users can only search public + their own.
func (s *ToolsetService) Search(ctx context.Context, query, mode string, limit int, userID string, isAdmin bool) (*ToolsetSearchResult, error) {
	switch mode {
	case "semantic":
		if s.embedding == nil || !s.embedding.IsEnabled() {
			return nil, fmt.Errorf("%w: semantic search is not enabled", ErrNotConfigured)
		}
		emb, err := s.embedding.Generate(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embedding: %w", err)
		}
		results, err := s.facade.SemanticSearch(emb, limit, s.scoreThreshold, userID, isAdmin)
		if err != nil {
			return nil, err
		}
		return &ToolsetSearchResult{Toolsets: results, Total: len(results), Mode: "semantic"}, nil

	default: // keyword
		toolsets, err := s.facade.Search(query, limit, userID, isAdmin)
		if err != nil {
			return nil, err
		}
		return &ToolsetSearchResult{Toolsets: toolsets, Total: len(toolsets), Mode: "keyword"}, nil
	}
}

// --- Private Helpers ---

// generateEmbedding generates embedding vector from name and description.
// Returns the vector on success, or an error if embedding is not available or generation fails.
func (s *ToolsetService) generateEmbedding(ctx context.Context, name, description string) (pgvector.Vector, error) {
	if s.embedding == nil || !s.embedding.IsEnabled() {
		return pgvector.Vector{}, fmt.Errorf("embedding service not available")
	}

	emb, err := s.embedding.GenerateForTool(ctx, name, description)
	if err != nil {
		return pgvector.Vector{}, fmt.Errorf("failed to generate embedding: %w", err)
	}

	return pgvector.NewVector(emb), nil
}
