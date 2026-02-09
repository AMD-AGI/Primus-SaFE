// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/embedding"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/storage"
)

// ToolService handles CRUD operations and like/unlike for tools
type ToolService struct {
	facade    *database.ToolFacade
	storage   storage.Storage
	embedding *embedding.Service
}

// NewToolService creates a new ToolService
func NewToolService(
	facade *database.ToolFacade,
	storageSvc storage.Storage,
	embeddingSvc *embedding.Service,
) *ToolService {
	return &ToolService{
		facade:    facade,
		storage:   storageSvc,
		embedding: embeddingSvc,
	}
}

// --- Input/Output Types ---

// ToolWithLikeStatus extends Tool with is_liked field for API responses
type ToolWithLikeStatus struct {
	model.Tool
	IsLiked bool `json:"is_liked"`
}

// ListInput represents input for listing tools
type ListInput struct {
	Type      string
	Status    string
	Owner     string
	SortField string
	SortOrder string
	Offset    int
	Limit     int
	UserID    string
}

// ListResult represents the result of listing tools
type ListResult struct {
	Tools  []ToolWithLikeStatus
	Total  int64
	Offset int
	Limit  int
}

// CreateMCPInput represents input for creating an MCP server
type CreateMCPInput struct {
	Name        string
	DisplayName string
	Description string
	Tags        []string
	IconURL     string
	Author      string
	Config      map[string]interface{}
	IsPublic    *bool
	UserID      string
	Username    string
}

// UpdateToolInput represents input for updating a tool
type UpdateToolInput struct {
	Name        string
	DisplayName string
	Description string
	Tags        []string
	IconURL     string
	Author      string
	Config      map[string]interface{}
	IsPublic    *bool
	Status      string
}

// --- Service Methods ---

// List retrieves tools with filters, sorting, and like status
func (s *ToolService) List(ctx context.Context, input ListInput) (*ListResult, error) {
	// Validate sort field
	validSortFields := map[string]bool{
		"created_at": true, "updated_at": true,
		"run_count": true, "download_count": true, "like_count": true,
	}
	if !validSortFields[input.SortField] {
		input.SortField = "created_at"
	}
	if input.SortOrder != "asc" && input.SortOrder != "desc" {
		input.SortOrder = "desc"
	}

	ownerOnly := input.Owner == "me"
	tools, total, err := s.facade.List(input.Type, input.Status, input.SortField, input.SortOrder, input.Offset, input.Limit, input.UserID, ownerOnly)
	if err != nil {
		return nil, err
	}

	// Get like status
	likedMap := make(map[int64]bool)
	if input.UserID != "" && len(tools) > 0 {
		toolIDs := make([]int64, len(tools))
		for i, t := range tools {
			toolIDs[i] = t.ID
		}
		likedMap, _ = s.facade.GetLikedToolIDs(input.UserID, toolIDs)
	}

	toolsWithLike := make([]ToolWithLikeStatus, len(tools))
	for i, t := range tools {
		toolsWithLike[i] = ToolWithLikeStatus{
			Tool:    t,
			IsLiked: likedMap[t.ID],
		}
	}

	return &ListResult{
		Tools:  toolsWithLike,
		Total:  total,
		Offset: input.Offset,
		Limit:  input.Limit,
	}, nil
}

// GetTool retrieves a tool by ID with access control and like status
func (s *ToolService) GetTool(ctx context.Context, id int64, userID string) (*ToolWithLikeStatus, error) {
	tool, err := s.facade.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("tool %w", ErrNotFound)
	}

	if !tool.IsPublic && tool.OwnerUserID != userID {
		return nil, ErrAccessDenied
	}

	isLiked := false
	if userID != "" {
		isLiked, _ = s.facade.IsLiked(id, userID)
	}

	return &ToolWithLikeStatus{Tool: *tool, IsLiked: isLiked}, nil
}

// CreateMCP creates a new MCP server tool
func (s *ToolService) CreateMCP(ctx context.Context, input CreateMCPInput) (*model.Tool, error) {
	displayName := input.DisplayName
	if displayName == "" {
		displayName = input.Name
	}

	author := input.Author
	if author == "" {
		author = input.Username
	}

	isPublic := true
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	tool := &model.Tool{
		Type:        model.AppTypeMCP,
		Name:        input.Name,
		DisplayName: displayName,
		Description: input.Description,
		Tags:        input.Tags,
		IconURL:     input.IconURL,
		Author:      author,
		Config:      model.AppConfig(input.Config),
		OwnerUserID: input.UserID,
		IsPublic:    isPublic,
		Status:      model.AppStatusActive,
	}

	if err := s.facade.Create(tool); err != nil {
		return nil, err
	}

	// Generate embedding synchronously
	s.generateEmbedding(ctx, tool)

	return tool, nil
}

// UpdateTool updates an existing tool with access control
func (s *ToolService) UpdateTool(ctx context.Context, id int64, input UpdateToolInput, userID string) (*model.Tool, error) {
	tool, err := s.facade.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("tool %w", ErrNotFound)
	}

	if tool.OwnerUserID != userID {
		return nil, fmt.Errorf("%w: only owner can update", ErrAccessDenied)
	}

	needReEmbed := false

	// Update name if provided (with uniqueness check)
	if input.Name != "" && input.Name != tool.Name {
		existing, err := s.facade.GetByTypeAndName(tool.Type, input.Name)
		if err == nil && existing.ID != tool.ID {
			return nil, fmt.Errorf("name '%s' already exists for type '%s'", input.Name, tool.Type)
		}
		tool.Name = input.Name
		needReEmbed = true
	}

	// Update fields if provided
	if input.DisplayName != "" {
		tool.DisplayName = input.DisplayName
	}
	if input.Description != "" {
		tool.Description = input.Description
		needReEmbed = true
	}
	if input.Tags != nil {
		tool.Tags = input.Tags
	}
	if input.IconURL != "" {
		tool.IconURL = input.IconURL
	}
	if input.Author != "" {
		tool.Author = input.Author
	}
	if input.IsPublic != nil {
		tool.IsPublic = *input.IsPublic
	}
	if input.Status != "" {
		tool.Status = input.Status
	}

	// Handle config update for skill (content update)
	if input.Config != nil {
		if tool.Type == model.AppTypeSkill {
			if content, ok := input.Config["content"].(string); ok && content != "" {
				s3Key := tool.GetSkillS3Key()
				if s3Key == "" {
					s3Key = fmt.Sprintf("skills/%d/SKILL.md", time.Now().UnixNano())
					tool.Config = map[string]interface{}{
						"s3_key":    s3Key,
						"is_prefix": false,
					}
				}
				if s.storage != nil {
					if err := s.storage.UploadBytes(ctx, s3Key, []byte(content)); err != nil {
						return nil, fmt.Errorf("failed to upload skill content: %w", err)
					}
				}
			} else {
				tool.Config = input.Config
			}
		} else {
			tool.Config = input.Config
		}
	}

	if err := s.facade.Update(tool); err != nil {
		return nil, err
	}

	// Re-generate embedding if name or description changed
	if needReEmbed {
		s.generateEmbedding(ctx, tool)
	}

	return tool, nil
}

// DeleteTool deletes a tool with access control
func (s *ToolService) DeleteTool(ctx context.Context, id int64, userID string) error {
	tool, err := s.facade.GetByID(id)
	if err != nil {
		return fmt.Errorf("tool %w", ErrNotFound)
	}

	if tool.OwnerUserID != userID {
		return fmt.Errorf("%w: only owner can delete", ErrAccessDenied)
	}

	// Delete S3 content for skill
	if tool.Type == model.AppTypeSkill && s.storage != nil {
		if s3Key := tool.GetSkillS3Key(); s3Key != "" {
			_ = s.storage.Delete(ctx, s3Key)
		}
	}

	return s.facade.Delete(id)
}

// LikeTool likes a tool and returns the new like count
func (s *ToolService) LikeTool(ctx context.Context, toolID int64, userID string) (int, error) {
	if userID == "" {
		return 0, fmt.Errorf("user id required for liking")
	}

	_, err := s.facade.GetByID(toolID)
	if err != nil {
		return 0, fmt.Errorf("tool %w", ErrNotFound)
	}

	if err := s.facade.Like(toolID, userID); err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return 0, ErrAlreadyLiked
		}
		return 0, err
	}

	count, _ := s.facade.GetLikeCount(toolID)
	return count, nil
}

// UnlikeTool unlikes a tool and returns the new like count
func (s *ToolService) UnlikeTool(ctx context.Context, toolID int64, userID string) (int, error) {
	if userID == "" {
		return 0, fmt.Errorf("user id required for unliking")
	}

	_, err := s.facade.GetByID(toolID)
	if err != nil {
		return 0, fmt.Errorf("tool %w", ErrNotFound)
	}

	if err := s.facade.Unlike(toolID, userID); err != nil {
		return 0, err
	}

	count, _ := s.facade.GetLikeCount(toolID)
	return count, nil
}

// --- Private Helpers ---

// generateEmbedding generates embedding for a tool synchronously
func (s *ToolService) generateEmbedding(ctx context.Context, tool *model.Tool) {
	if s.embedding == nil || !s.embedding.IsEnabled() {
		return
	}

	emb, err := s.embedding.GenerateForTool(ctx, tool.Name, tool.Description)
	if err != nil {
		fmt.Printf("Failed to generate embedding for tool %d: %v\n", tool.ID, err)
		return
	}

	if err := s.facade.UpdateEmbedding(tool.ID, emb); err != nil {
		fmt.Printf("Failed to update embedding for tool %d: %v\n", tool.ID, err)
	}
}

// CloneInput represents input for cloning a tool
type CloneInput struct {
	SourceID int64
	UserID   string
	Username string
}

// Clone creates a copy of an existing tool
func (s *ToolService) Clone(ctx context.Context, input CloneInput) (*model.Tool, error) {
	// 1. Get source tool
	source, err := s.facade.GetByID(input.SourceID)
	if err != nil {
		return nil, fmt.Errorf("tool %w", ErrNotFound)
	}

	// Check access: must be public or owned by user
	if !source.IsPublic && source.OwnerUserID != input.UserID {
		return nil, ErrAccessDenied
	}

	// 2. Generate unique name with -copy suffix
	newName := s.generateUniqueCopyName(source.Type, source.Name)

	// 3. Create new tool with copied properties
	newTool := &model.Tool{
		Type:        source.Type,
		Name:        newName,
		DisplayName: source.DisplayName,
		Description: source.Description,
		Tags:        source.Tags,
		IconURL:     source.IconURL,
		Author:      input.Username, // New author is the cloner
		Config:      source.Config,
		SkillSource: source.SkillSource,
		OwnerUserID: input.UserID, // New owner
		IsPublic:    false,        // Cloned tools default to private
		Status:      model.AppStatusActive,
	}

	// 4. For skills, copy S3 files
	if source.Type == model.AppTypeSkill && s.storage != nil {
		newConfig, err := s.copySkillFiles(ctx, source.Config, newName)
		if err != nil {
			fmt.Printf("Warning: failed to copy skill files: %v\n", err)
			// Continue with original config if copy fails
		} else {
			newTool.Config = newConfig
		}
	}

	// 5. Create the new tool
	if err := s.facade.Create(newTool); err != nil {
		return nil, fmt.Errorf("failed to create cloned tool: %w", err)
	}

	// 6. Generate embedding
	s.generateEmbedding(ctx, newTool)

	return newTool, nil
}

// generateUniqueCopyName generates a unique name with -copy suffix
func (s *ToolService) generateUniqueCopyName(toolType, baseName string) string {
	// Try baseName-copy first
	copyName := baseName + "-copy"
	_, err := s.facade.GetByTypeAndName(toolType, copyName)
	if err != nil {
		// Name is available
		return copyName
	}

	// Try baseName-copy-1, baseName-copy-2, etc.
	for i := 1; i <= 100; i++ {
		copyName = fmt.Sprintf("%s-copy-%d", baseName, i)
		_, err := s.facade.GetByTypeAndName(toolType, copyName)
		if err != nil {
			return copyName
		}
	}

	// Fallback: use timestamp
	return fmt.Sprintf("%s-copy-%d", baseName, time.Now().UnixNano())
}

// copySkillFiles copies S3 files for a skill
func (s *ToolService) copySkillFiles(ctx context.Context, sourceConfig model.AppConfig, newName string) (model.AppConfig, error) {
	s3Key, ok := sourceConfig["s3_key"].(string)
	if !ok || s3Key == "" {
		return sourceConfig, nil
	}

	isPrefix, _ := sourceConfig["is_prefix"].(bool)

	// Generate new S3 key
	timestamp := time.Now().UnixNano()
	newS3KeyBase := fmt.Sprintf("skills/%s/%d", newName, timestamp)

	if isPrefix {
		// Copy all files under the prefix
		oldPrefix := strings.TrimSuffix(s3Key, "/SKILL.md")
		if err := s.storage.CopyPrefix(ctx, oldPrefix, newS3KeyBase); err != nil {
			return nil, err
		}
	} else {
		// Copy single file
		newS3Key := newS3KeyBase + "/SKILL.md"
		if err := s.storage.Copy(ctx, s3Key, newS3Key); err != nil {
			return nil, err
		}
	}

	return model.AppConfig{
		"s3_key":    newS3KeyBase + "/SKILL.md",
		"is_prefix": isPrefix,
	}, nil
}
