// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
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
	IsAdmin   bool
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
	tools, total, err := s.facade.List(input.Type, input.Status, input.SortField, input.SortOrder, input.Offset, input.Limit, input.UserID, ownerOnly, input.IsAdmin)
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

// UpdateTool updates an existing tool with access control.
// Admins can update any tool; regular users can only update their own.
func (s *ToolService) UpdateTool(ctx context.Context, id int64, input UpdateToolInput, userID string, isAdmin bool) (*model.Tool, error) {
	tool, err := s.facade.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("tool %w", ErrNotFound)
	}

	if !isAdmin && tool.OwnerUserID != userID {
		return nil, fmt.Errorf("%w: only owner or admin can update", ErrAccessDenied)
	}

	// Update fields if provided
	needReEmbed := false
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

// DeleteTool deletes a tool with access control.
// Admins can delete any tool; regular users can only delete their own.
func (s *ToolService) DeleteTool(ctx context.Context, id int64, userID string, isAdmin bool) error {
	tool, err := s.facade.GetByID(id)
	if err != nil {
		return fmt.Errorf("tool %w", ErrNotFound)
	}

	if !isAdmin && tool.OwnerUserID != userID {
		return fmt.Errorf("%w: only owner or admin can delete", ErrAccessDenied)
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

// UploadIcon uploads an icon file to S3 and returns the URL
func (s *ToolService) UploadIcon(ctx context.Context, userID, filename string, file io.Reader) (string, error) {
	if s.storage == nil {
		return "", ErrNotConfigured
	}

	// Generate unique S3 key: icons/{userID}/{timestamp}.{ext}
	timestamp := time.Now().Unix()
	ext := filepath.Ext(filename)
	key := fmt.Sprintf("icons/%s/%d%s", userID, timestamp, ext)

	// Upload to S3
	if err := s.storage.Upload(ctx, key, file); err != nil {
		return "", fmt.Errorf("failed to upload icon: %w", err)
	}

	// Get presigned URL (or public URL if available)
	url, err := s.storage.GetURL(ctx, key)
	if err != nil {
		return "", fmt.Errorf("failed to get icon URL: %w", err)
	}

	return url, nil
}

// GetToolContent retrieves the raw SKILL.md content for a skill
func (s *ToolService) GetToolContent(ctx context.Context, toolID int64, userID string) (string, error) {
	// Get tool and check access
	tool, err := s.facade.GetByID(toolID)
	if err != nil {
		return "", fmt.Errorf("tool %w", ErrNotFound)
	}

	// Access control
	if !tool.IsPublic && tool.OwnerUserID != userID {
		return "", ErrAccessDenied
	}

	// Only skills have content
	if tool.Type != "skill" {
		return "", fmt.Errorf("only skills have content")
	}

	// Get S3 key from config
	s3Key, ok := tool.Config["s3_key"].(string)
	if !ok || s3Key == "" {
		return "", fmt.Errorf("s3_key not found in config")
	}

	// Download from S3
	if s.storage == nil {
		return "", ErrNotConfigured
	}

	// New imports with is_prefix=true store the directory prefix (e.g. "skills/name/123/").
	// In that case, append "SKILL.md" to get the actual file path.
	downloadKey := s3Key
	if strings.HasSuffix(s3Key, "/") {
		downloadKey = s3Key + "SKILL.md"
	}

	content, err := s.storage.DownloadBytes(ctx, downloadKey)
	if err != nil {
		return "", fmt.Errorf("failed to download skill content: %w", err)
	}

	return string(content), nil
}
