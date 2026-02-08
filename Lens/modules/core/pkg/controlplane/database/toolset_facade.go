// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"errors"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// ToolsetFacade provides database operations for toolsets
type ToolsetFacade struct {
	db *gorm.DB
}

// NewToolsetFacade creates a new ToolsetFacade
func NewToolsetFacade(db *gorm.DB) *ToolsetFacade {
	return &ToolsetFacade{db: db}
}

// Create creates a new toolset.
// If Embedding is zero-value (empty), it will be omitted to avoid pgvector errors.
func (f *ToolsetFacade) Create(toolset *model.Toolset) error {
	if len(toolset.Embedding.Slice()) == 0 {
		return f.db.Omit("Embedding").Create(toolset).Error
	}
	return f.db.Create(toolset).Error
}

// GetByID retrieves a toolset by ID
func (f *ToolsetFacade) GetByID(id int64) (*model.Toolset, error) {
	var toolset model.Toolset
	err := f.db.Where("id = ?", id).First(&toolset).Error
	if err != nil {
		return nil, err
	}
	return &toolset, nil
}

// List retrieves toolsets with optional filters and sorting
func (f *ToolsetFacade) List(sortField, sortOrder string, offset, limit int, userID string, ownerOnly bool) ([]model.Toolset, int64, error) {
	var toolsets []model.Toolset
	var total int64

	query := f.db.Model(&model.Toolset{})

	// Access control (same pattern as ToolFacade)
	if ownerOnly && userID != "" {
		query = query.Where("owner_user_id = ?", userID)
	} else if userID != "" {
		query = query.Where("is_public = ? OR owner_user_id = ?", true, userID)
	} else {
		query = query.Where("is_public = ?", true)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	orderClause := fmt.Sprintf("%s %s", sortField, sortOrder)
	err = query.Order(orderClause).Offset(offset).Limit(limit).Find(&toolsets).Error
	if err != nil {
		return nil, 0, err
	}

	return toolsets, total, nil
}

// Update updates a toolset.
// If Embedding is zero-value (empty), it will be omitted to preserve existing embedding.
func (f *ToolsetFacade) Update(toolset *model.Toolset) error {
	if len(toolset.Embedding.Slice()) == 0 {
		return f.db.Omit("Embedding").Save(toolset).Error
	}
	return f.db.Save(toolset).Error
}

// Delete deletes a toolset by ID (soft delete)
func (f *ToolsetFacade) Delete(id int64) error {
	return f.db.Delete(&model.Toolset{}, id).Error
}

// --- ToolsetTool operations ---

// AddTools adds tools to a toolset and updates tool_count
func (f *ToolsetFacade) AddTools(toolsetID int64, toolIDs []int64) (int, error) {
	added := 0
	err := f.db.Transaction(func(tx *gorm.DB) error {
		// Get current max sort_order
		var maxOrder int
		tx.Model(&model.ToolsetTool{}).
			Where("toolset_id = ?", toolsetID).
			Select("COALESCE(MAX(sort_order), 0)").
			Scan(&maxOrder)

		for _, toolID := range toolIDs {
			record := &model.ToolsetTool{
				ToolsetID: toolsetID,
				ToolID:    toolID,
				SortOrder: maxOrder + added + 1,
			}
			result := tx.Create(record)
			if result.Error != nil {
				// Skip unique constraint violations (duplicate tool in toolset)
				var pgErr *pgconn.PgError
				if errors.As(result.Error, &pgErr) && pgErr.Code == "23505" {
					continue
				}
				// Any other error should abort the transaction
				return result.Error
			}
			added++
		}

		if added > 0 {
			// Update denormalized tool_count
			return tx.Model(&model.Toolset{}).Where("id = ?", toolsetID).
				Update("tool_count", gorm.Expr("tool_count + ?", added)).Error
		}
		return nil
	})
	return added, err
}

// RemoveTool removes a tool from a toolset and updates tool_count
func (f *ToolsetFacade) RemoveTool(toolsetID, toolID int64) error {
	return f.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("toolset_id = ? AND tool_id = ?", toolsetID, toolID).
			Delete(&model.ToolsetTool{})
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected > 0 {
			return tx.Model(&model.Toolset{}).Where("id = ?", toolsetID).
				Update("tool_count", gorm.Expr("tool_count - 1")).Error
		}
		return nil
	})
}

// GetTools retrieves all active tools in a toolset, ordered by sort_order
func (f *ToolsetFacade) GetTools(toolsetID int64) ([]model.Tool, error) {
	var tools []model.Tool
	err := f.db.
		Joins("JOIN toolset_tools ON toolset_tools.tool_id = tools.id").
		Where("toolset_tools.toolset_id = ?", toolsetID).
		Where("tools.deleted_at IS NULL").
		Where("tools.status = ?", model.AppStatusActive).
		Order("toolset_tools.sort_order ASC").
		Find(&tools).Error
	return tools, err
}

// Search performs a keyword search on toolsets
func (f *ToolsetFacade) Search(query string, limit int, userID string) ([]model.Toolset, error) {
	var toolsets []model.Toolset

	dbQuery := f.db.Model(&model.Toolset{}).
		Where("name ILIKE ? OR description ILIKE ? OR display_name ILIKE ?",
			"%"+query+"%", "%"+query+"%", "%"+query+"%")

	// Access control
	if userID != "" {
		dbQuery = dbQuery.Where("is_public = ? OR owner_user_id = ?", true, userID)
	} else {
		dbQuery = dbQuery.Where("is_public = ?", true)
	}

	err := dbQuery.Order("tool_count DESC").Limit(limit).Find(&toolsets).Error
	return toolsets, err
}

// SemanticSearch performs a vector similarity search on toolsets
func (f *ToolsetFacade) SemanticSearch(embedding []float32, limit int, scoreThreshold float64, userID string) ([]ToolsetWithScore, error) {
	var results []ToolsetWithScore

	embStr := "["
	for i, v := range embedding {
		if i > 0 {
			embStr += ","
		}
		embStr += fmt.Sprintf("%f", v)
	}
	embStr += "]"

	query := f.db.Model(&model.Toolset{}).
		Select("*, 1 - (embedding <=> ?) as score", embStr).
		Where("embedding IS NOT NULL")

	// Access control
	if userID != "" {
		query = query.Where("is_public = ? OR owner_user_id = ?", true, userID)
	} else {
		query = query.Where("is_public = ?", true)
	}

	if scoreThreshold > 0 {
		query = query.Where("1 - (embedding <=> ?) > ?", embStr, scoreThreshold)
	}

	err := query.Order("score DESC").Limit(limit).Find(&results).Error
	return results, err
}

// ToolsetWithScore represents a toolset with similarity score
type ToolsetWithScore struct {
	model.Toolset
	Score float64 `json:"score"`
}
