// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"errors"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// ToolFacade provides database operations for tools
type ToolFacade struct {
	db *gorm.DB
}

// NewToolFacade creates a new ToolFacade
func NewToolFacade(db *gorm.DB) *ToolFacade {
	return &ToolFacade{db: db}
}

// Create creates a new tool
func (f *ToolFacade) Create(tool *model.Tool) error {
	// Check if an active (non-deleted) tool with the same type+name already exists
	_, err := f.GetByTypeAndName(tool.Type, tool.Name)
	if err == nil {
		return fmt.Errorf("tool %s/%s already exists", tool.Type, tool.Name)
	}
	// Only proceed if the error is "record not found"
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check existing tool: %w", err)
	}

	// Hard-delete any soft-deleted record with the same type+name to avoid
	// UNIQUE(type, name) constraint violation. Soft-deleted rows still occupy
	// the unique index, so we must remove them before inserting a new row.
	f.db.Unscoped().
		Where("type = ? AND name = ? AND deleted_at IS NOT NULL", tool.Type, tool.Name).
		Delete(&model.Tool{})

	// Omit embedding field to avoid pgvector empty vector error
	// Embedding will be updated asynchronously via UpdateEmbedding
	return f.db.Omit("Embedding").Create(tool).Error
}

// GetByID retrieves a tool by ID
func (f *ToolFacade) GetByID(id int64) (*model.Tool, error) {
	var tool model.Tool
	err := f.db.Where("id = ?", id).First(&tool).Error
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

// GetByTypeAndName retrieves a tool by type and name
func (f *ToolFacade) GetByTypeAndName(toolType, name string) (*model.Tool, error) {
	var tool model.Tool
	err := f.db.Where("type = ? AND name = ?", toolType, name).First(&tool).Error
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

// List retrieves tools with optional filters and sorting
// userID is used for access control: returns public tools + tools owned by the user
// ownerOnly: if true, only returns tools owned by the user (for "My Tools" view)
func (f *ToolFacade) List(toolType, status, sortField, sortOrder string, offset, limit int, userID string, ownerOnly bool) ([]model.Tool, int64, error) {
	var tools []model.Tool
	var total int64

	query := f.db.Model(&model.Tool{})

	if toolType != "" {
		query = query.Where("type = ?", toolType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Access control
	if ownerOnly && userID != "" {
		// Only return tools owned by the user
		query = query.Where("owner_user_id = ?", userID)
	} else if userID != "" {
		// Return public tools + tools owned by current user
		query = query.Where("is_public = ? OR owner_user_id = ?", true, userID)
	} else {
		// Anonymous users can only see public tools
		query = query.Where("is_public = ?", true)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// Build order clause
	orderClause := fmt.Sprintf("%s %s", sortField, sortOrder)
	err = query.Order(orderClause).Offset(offset).Limit(limit).Find(&tools).Error
	if err != nil {
		return nil, 0, err
	}

	return tools, total, nil
}

// Update updates a tool
// Omit embedding field to avoid pgvector empty vector error
// Embedding should be updated via UpdateEmbedding method
func (f *ToolFacade) Update(tool *model.Tool) error {
	return f.db.Omit("Embedding").Save(tool).Error
}

// Delete deletes a tool by ID
func (f *ToolFacade) Delete(id int64) error {
	return f.db.Delete(&model.Tool{}, id).Error
}

// IncrementRunCount increments the run count for a tool
func (f *ToolFacade) IncrementRunCount(id int64) error {
	return f.db.Model(&model.Tool{}).Where("id = ?", id).
		Update("run_count", gorm.Expr("run_count + 1")).Error
}

// IncrementDownloadCount increments the download count for a tool
func (f *ToolFacade) IncrementDownloadCount(id int64) error {
	return f.db.Model(&model.Tool{}).Where("id = ?", id).
		Update("download_count", gorm.Expr("download_count + 1")).Error
}

// Search performs a text search on tools (keyword mode)
func (f *ToolFacade) Search(query string, toolType string, limit int) ([]model.Tool, error) {
	var tools []model.Tool

	dbQuery := f.db.Model(&model.Tool{}).
		Where("status = ?", model.AppStatusActive).
		Where("name ILIKE ? OR description ILIKE ? OR display_name ILIKE ?",
			"%"+query+"%", "%"+query+"%", "%"+query+"%")

	if toolType != "" {
		dbQuery = dbQuery.Where("type = ?", toolType)
	}

	err := dbQuery.Order("run_count DESC").Limit(limit).Find(&tools).Error
	return tools, err
}

// ToolWithScore represents a tool with similarity score
type ToolWithScore struct {
	model.Tool
	Score float64 `json:"score"`
}

// SemanticSearch performs a vector similarity search on tools
// scoreThreshold: minimum similarity score (0-1), results below this threshold are filtered out.
// Pass 0 to disable threshold filtering.
func (f *ToolFacade) SemanticSearch(embedding []float32, toolType string, limit int, scoreThreshold float64) ([]ToolWithScore, error) {
	var results []ToolWithScore

	// Build embedding string for pgvector
	embStr := "["
	for i, v := range embedding {
		if i > 0 {
			embStr += ","
		}
		embStr += fmt.Sprintf("%f", v)
	}
	embStr += "]"

	query := f.db.Model(&model.Tool{}).
		Select("*, 1 - (embedding <=> ?) as score", embStr).
		Where("status = ?", model.AppStatusActive).
		Where("embedding IS NOT NULL")

	if toolType != "" {
		query = query.Where("type = ?", toolType)
	}

	// Apply score threshold filter
	if scoreThreshold > 0 {
		query = query.Where("1 - (embedding <=> ?) > ?", embStr, scoreThreshold)
	}

	err := query.Order("score DESC").Limit(limit).Find(&results).Error
	return results, err
}

// HybridSearch combines keyword and semantic search
func (f *ToolFacade) HybridSearch(keyword string, embedding []float32, toolType string, limit int, scoreThreshold float64) ([]ToolWithScore, error) {
	// Get keyword results
	keywordTools, err := f.Search(keyword, toolType, limit*2)
	if err != nil {
		return nil, err
	}

	// Get semantic results (with score threshold)
	semanticResults, err := f.SemanticSearch(embedding, toolType, limit*2, scoreThreshold)
	if err != nil {
		return nil, err
	}

	// Combine and deduplicate
	seen := make(map[int64]bool)
	var results []ToolWithScore

	// Add semantic results first (higher priority)
	for _, r := range semanticResults {
		if !seen[r.ID] {
			seen[r.ID] = true
			results = append(results, r)
		}
	}

	// Add keyword results
	for _, t := range keywordTools {
		if !seen[t.ID] {
			seen[t.ID] = true
			results = append(results, ToolWithScore{Tool: t, Score: 0.5})
		}
	}

	// Limit results
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// UpdateEmbedding updates the embedding for a tool
func (f *ToolFacade) UpdateEmbedding(id int64, embedding []float32) error {
	// Build embedding string for pgvector
	embStr := "["
	for i, v := range embedding {
		if i > 0 {
			embStr += ","
		}
		embStr += fmt.Sprintf("%f", v)
	}
	embStr += "]"

	return f.db.Model(&model.Tool{}).Where("id = ?", id).
		Update("embedding", gorm.Expr("?::vector", embStr)).Error
}

// Like adds a like from a user to a tool
func (f *ToolFacade) Like(toolID int64, userID string) error {
	return f.db.Transaction(func(tx *gorm.DB) error {
		// Try to insert like record (will fail if already exists due to unique constraint)
		like := &model.ToolLike{ToolID: toolID, UserID: userID}
		result := tx.Create(like)
		if result.Error != nil {
			// If it's a unique constraint violation, the user already liked
			return result.Error
		}

		// Increment like count
		return tx.Model(&model.Tool{}).Where("id = ?", toolID).
			Update("like_count", gorm.Expr("like_count + 1")).Error
	})
}

// Unlike removes a like from a user to a tool
func (f *ToolFacade) Unlike(toolID int64, userID string) error {
	return f.db.Transaction(func(tx *gorm.DB) error {
		// Delete like record
		result := tx.Where("tool_id = ? AND user_id = ?", toolID, userID).
			Delete(&model.ToolLike{})
		if result.Error != nil {
			return result.Error
		}

		// Only decrement if a record was actually deleted
		if result.RowsAffected > 0 {
			return tx.Model(&model.Tool{}).Where("id = ?", toolID).
				Update("like_count", gorm.Expr("like_count - 1")).Error
		}
		return nil
	})
}

// IsLiked checks if a user has liked a tool
func (f *ToolFacade) IsLiked(toolID int64, userID string) (bool, error) {
	var count int64
	err := f.db.Model(&model.ToolLike{}).
		Where("tool_id = ? AND user_id = ?", toolID, userID).
		Count(&count).Error
	return count > 0, err
}

// GetLikeCount returns the like count for a tool
func (f *ToolFacade) GetLikeCount(toolID int64) (int, error) {
	var tool model.Tool
	err := f.db.Select("like_count").Where("id = ?", toolID).First(&tool).Error
	if err != nil {
		return 0, err
	}
	return tool.LikeCount, nil
}

// GetLikedToolIDs returns a map of tool IDs that the user has liked
// This is used for batch checking like status in list endpoints
func (f *ToolFacade) GetLikedToolIDs(userID string, toolIDs []int64) (map[int64]bool, error) {
	if userID == "" || len(toolIDs) == 0 {
		return make(map[int64]bool), nil
	}

	var likes []model.ToolLike
	err := f.db.Where("user_id = ? AND tool_id IN ?", userID, toolIDs).Find(&likes).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int64]bool)
	for _, like := range likes {
		result[like.ToolID] = true
	}
	return result, nil
}
