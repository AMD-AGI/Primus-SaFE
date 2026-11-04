/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"

	"github.com/lib/pq"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TPostTrainingExperiment = "post_training_experiment"
)

// PostTrainingExperiment is an A/B experiment: a baseline and up to four variants of one train type, each a run
// that records experiment_id and variant_name. Variants is a JSON array of each variant's name and the fields it
// changes from the baseline. The experiment's status is derived from its runs and not stored.
type PostTrainingExperiment struct {
	ID              int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id" db:"id"`
	ExperimentID    string `gorm:"column:experiment_id" json:"experimentId" db:"experiment_id"`
	Workspace       string `gorm:"column:workspace" json:"workspace" db:"workspace"`
	DisplayName     string `gorm:"column:display_name" json:"displayName" db:"display_name"`
	TrainType       string `gorm:"column:train_type" json:"trainType" db:"train_type"`
	BaselineVariant string `gorm:"column:baseline_variant" json:"baselineVariant" db:"baseline_variant"`
	Variants        string `gorm:"column:variants" json:"variants" db:"variants"`
	// Seed is the training and rollout seed every variant shares.
	Seed      int         `gorm:"column:seed" json:"seed" db:"seed"`
	UserID    string      `gorm:"column:user_id" json:"userId" db:"user_id"`
	UserName  string      `gorm:"column:user_name" json:"userName" db:"user_name"`
	CreatedAt pq.NullTime `gorm:"column:created_at;autoCreateTime" json:"createdAt" db:"created_at"`
	UpdatedAt pq.NullTime `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt" db:"updated_at"`
	IsDeleted bool        `gorm:"column:is_deleted" json:"-" db:"is_deleted"`
}

// TableName sets the SQL table name for PostTrainingExperiment.
func (PostTrainingExperiment) TableName() string {
	return TPostTrainingExperiment
}

// PostTrainingExperimentFilter selects experiments for ListPostTrainingExperiments. Zero-valued fields are ignored.
type PostTrainingExperimentFilter struct {
	Workspace     string
	ExperimentIDs []string
	Limit         int
	Offset        int
}

// CreatePostTrainingExperiment inserts an experiment; experiment_id must be unique.
func (c *Client) CreatePostTrainingExperiment(ctx context.Context, e *PostTrainingExperiment) error {
	if e == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Create(e).Error
}

// GetPostTrainingExperiment fetches a non-deleted experiment. It returns gorm.ErrRecordNotFound when there is none.
func (c *Client) GetPostTrainingExperiment(ctx context.Context, experimentID string) (*PostTrainingExperiment, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var e PostTrainingExperiment
	if err = db.WithContext(ctx).Where("experiment_id = ? AND is_deleted = ?", experimentID, false).
		First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

// ListPostTrainingExperiments returns the non-deleted experiments matching filter, newest first, and their total
// count.
func (c *Client) ListPostTrainingExperiments(ctx context.Context,
	filter PostTrainingExperimentFilter) ([]*PostTrainingExperiment, int64, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, 0, err
	}
	query := db.WithContext(ctx).Model(&PostTrainingExperiment{}).Where("is_deleted = ?", false)
	if filter.Workspace != "" {
		query = query.Where("workspace = ?", filter.Workspace)
	}
	if len(filter.ExperimentIDs) > 0 {
		query = query.Where("experiment_id IN ?", filter.ExperimentIDs)
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
	var out []*PostTrainingExperiment
	if err = query.Order("created_at DESC").Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// SetPostTrainingExperimentDeleted soft-deletes an experiment once its runs are deleted.
func (c *Client) SetPostTrainingExperimentDeleted(ctx context.Context, experimentID string) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Model(&PostTrainingExperiment{}).Where("experiment_id = ?", experimentID).
		Update("is_deleted", true).Error
}
