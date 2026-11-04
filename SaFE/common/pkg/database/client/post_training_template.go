/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"

	"github.com/lib/pq"
	"gorm.io/gorm"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TPostTrainingTemplate = "post_training_template"
)

// PostTrainingTemplate is a recipe template of a workspace: the framework, train type, method, spec, options,
// resources, evaluation and W&B settings, and optionally the base model and dataset, as JSON content. Every edit
// increments the version; runs record the version they used.
type PostTrainingTemplate struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id" db:"id"`
	TemplateID  string `gorm:"column:template_id" json:"templateId" db:"template_id"`
	Workspace   string `gorm:"column:workspace" json:"workspace" db:"workspace"`
	Name        string `gorm:"column:name" json:"name" db:"name"`
	Description string `gorm:"column:description" json:"description,omitempty" db:"description"`
	TrainType   string `gorm:"column:train_type" json:"trainType" db:"train_type"`
	Method      string `gorm:"column:method" json:"method" db:"method"`
	Framework   string `gorm:"column:framework;default:slime" json:"framework" db:"framework"`
	// Spec is the spec the template was written for; empty means any spec the model matches.
	Spec        string      `gorm:"column:spec" json:"spec,omitempty" db:"spec"`
	Version     int         `gorm:"column:version;default:1" json:"version" db:"version"`
	Content     string      `gorm:"column:content" json:"content" db:"content"`
	SourceRunID string      `gorm:"column:source_run_id" json:"sourceRunId,omitempty" db:"source_run_id"`
	UserID      string      `gorm:"column:user_id" json:"userId" db:"user_id"`
	UserName    string      `gorm:"column:user_name" json:"userName" db:"user_name"`
	CreatedAt   pq.NullTime `gorm:"column:created_at;autoCreateTime" json:"createdAt" db:"created_at"`
	UpdatedAt   pq.NullTime `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt" db:"updated_at"`
	IsDeleted   bool        `gorm:"column:is_deleted" json:"-" db:"is_deleted"`
}

// TableName sets the SQL table name for PostTrainingTemplate.
func (PostTrainingTemplate) TableName() string {
	return TPostTrainingTemplate
}

// PostTrainingTemplateFilter selects templates for ListPostTrainingTemplates. Zero-valued fields are ignored.
type PostTrainingTemplateFilter struct {
	Workspace string
	TrainType string
	Search    string // matches name ILIKE %search%
	Limit     int
	Offset    int
}

// CreatePostTrainingTemplate inserts a template; template_id must be unique.
func (c *Client) CreatePostTrainingTemplate(ctx context.Context, t *PostTrainingTemplate) error {
	if t == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Create(t).Error
}

// GetPostTrainingTemplate fetches a non-deleted template. It returns gorm.ErrRecordNotFound when there is none.
func (c *Client) GetPostTrainingTemplate(ctx context.Context, templateID string) (*PostTrainingTemplate, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var t PostTrainingTemplate
	if err = db.WithContext(ctx).Where("template_id = ? AND is_deleted = ?", templateID, false).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// ListPostTrainingTemplates returns the non-deleted templates matching filter, most recently updated first, and
// their total count.
func (c *Client) ListPostTrainingTemplates(ctx context.Context,
	filter PostTrainingTemplateFilter) ([]*PostTrainingTemplate, int64, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, 0, err
	}
	query := db.WithContext(ctx).Model(&PostTrainingTemplate{}).Where("is_deleted = ?", false)
	if filter.Workspace != "" {
		query = query.Where("workspace = ?", filter.Workspace)
	}
	if filter.TrainType != "" {
		query = query.Where("train_type = ?", filter.TrainType)
	}
	if filter.Search != "" {
		query = query.Where("name ILIKE ?", "%"+filter.Search+"%")
	}
	var total int64
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	var out []*PostTrainingTemplate
	if err = query.Order("COALESCE(updated_at, created_at) DESC").Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// UpdatePostTrainingTemplate sets the given columns of a non-deleted template and increments its version. It
// returns gorm.ErrRecordNotFound when no such template exists.
func (c *Client) UpdatePostTrainingTemplate(ctx context.Context, templateID string, fields map[string]interface{}) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	updates := map[string]interface{}{"version": gorm.Expr("version + 1")}
	for k, v := range fields {
		updates[k] = v
	}
	res := db.WithContext(ctx).Model(&PostTrainingTemplate{}).
		Where("template_id = ? AND is_deleted = ?", templateID, false).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// SetPostTrainingTemplateDeleted soft-deletes a template; runs keep the template id and version they recorded.
func (c *Client) SetPostTrainingTemplateDeleted(ctx context.Context, templateID string) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Model(&PostTrainingTemplate{}).Where("template_id = ?", templateID).
		Update("is_deleted", true).Error
}

// CountPostTrainingRunsByTemplate returns how many non-deleted runs of each template succeeded.
func (c *Client) CountPostTrainingRunsByTemplate(ctx context.Context, templateIDs []string) (map[string]int, error) {
	out := map[string]int{}
	if len(templateIDs) == 0 {
		return out, nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var rows []struct {
		TemplateID string `gorm:"column:template_id"`
		Succeeded  int    `gorm:"column:succeeded"`
	}
	if err = db.WithContext(ctx).Model(&PostTrainingRun{}).Select("template_id, COUNT(*) AS succeeded").
		Where("template_id IN ? AND status = ? AND is_deleted = ?", templateIDs, postTrainingRunSucceeded, false).
		Group("template_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.TemplateID] = r.Succeeded
	}
	return out, nil
}
