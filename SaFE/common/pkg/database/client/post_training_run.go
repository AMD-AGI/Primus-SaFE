/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TPostTrainingRun = "post_training_run"
)

// PostTrainingRun is one Post Training run. The apiserver inserts it on submission; after that only the
// Post Training controller updates it.
type PostTrainingRun struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id" db:"id"`
	RunID            string `gorm:"column:run_id" json:"runId" db:"run_id"`
	WorkloadID       string `gorm:"column:workload_id" json:"workloadId" db:"workload_id"`
	WorkloadUID      string `gorm:"column:workload_uid" json:"workloadUid,omitempty" db:"workload_uid"`
	DisplayName      string `gorm:"column:display_name" json:"displayName" db:"display_name"`
	Framework        string `gorm:"column:framework;default:slime" json:"framework" db:"framework"`
	FrameworkVersion string `gorm:"column:framework_version" json:"frameworkVersion,omitempty" db:"framework_version"`
	TrainType        string `gorm:"column:train_type" json:"trainType" db:"train_type"`
	Method           string `gorm:"column:method" json:"method" db:"method"`
	Algorithm        string `gorm:"column:algorithm" json:"algorithm,omitempty" db:"algorithm"`
	Workspace        string `gorm:"column:workspace" json:"workspace" db:"workspace"`
	Cluster          string `gorm:"column:cluster" json:"cluster" db:"cluster"`
	UserID           string `gorm:"column:user_id" json:"userId" db:"user_id"`
	UserName         string `gorm:"column:user_name" json:"userName" db:"user_name"`

	BaseModelID   string `gorm:"column:base_model_id" json:"baseModelId" db:"base_model_id"`
	BaseModelName string `gorm:"column:base_model_name" json:"baseModelName" db:"base_model_name"`
	DatasetID     string `gorm:"column:dataset_id" json:"datasetId" db:"dataset_id"`
	DatasetName   string `gorm:"column:dataset_name" json:"datasetName,omitempty" db:"dataset_name"`
	EvalDatasetID string `gorm:"column:eval_dataset_id" json:"evalDatasetId,omitempty" db:"eval_dataset_id"`
	Image         string `gorm:"column:image" json:"image,omitempty" db:"image"`
	NodeCount     int    `gorm:"column:node_count" json:"nodeCount" db:"node_count"`
	GpuPerNode    int    `gorm:"column:gpu_per_node" json:"gpuPerNode" db:"gpu_per_node"`
	Priority      int    `gorm:"column:priority" json:"priority" db:"priority"`
	Timeout       int    `gorm:"column:timeout" json:"timeout" db:"timeout"`
	OutputPath    string `gorm:"column:output_path" json:"outputPath,omitempty" db:"output_path"`
	OutputModelID string `gorm:"column:output_model_id" json:"outputModelId,omitempty" db:"output_model_id"`

	Status        string `gorm:"column:status;default:Pending" json:"status" db:"status"`
	Stage         string `gorm:"column:stage;default:queued" json:"stage" db:"stage"`
	FailedStage   string `gorm:"column:failed_stage" json:"failedStage,omitempty" db:"failed_stage"`
	Message       string `gorm:"column:message" json:"message,omitempty" db:"message"`
	QueuePosition int    `gorm:"column:queue_position" json:"queuePosition" db:"queue_position"`
	// StageHistory is a JSON array of {stage, time, detail} entries.
	StageHistory string `gorm:"column:stage_history" json:"stageHistory,omitempty" db:"stage_history"`
	// Recipe is the JSON recipe snapshot taken at submission; RecipeHash identifies it across runs.
	Recipe      string `gorm:"column:recipe" json:"recipe" db:"recipe"`
	RecipeHash  string `gorm:"column:recipe_hash" json:"recipeHash" db:"recipe_hash"`
	EvalEnabled bool   `gorm:"column:eval_enabled" json:"evalEnabled" db:"eval_enabled"`

	// MetricCursor and MetricSummary are JSON written together with each batch of metric points.
	MetricCursor  string `gorm:"column:metric_cursor" json:"-" db:"metric_cursor"`
	MetricSummary string `gorm:"column:metric_summary" json:"metricSummary,omitempty" db:"metric_summary"`
	MetricGap     bool   `gorm:"column:metric_gap" json:"metricGap" db:"metric_gap"`
	// ResultLine is the JSON of the last [SUCCESS] or [ERROR] marker the controller read.
	ResultLine        string `gorm:"column:result_line" json:"resultLine,omitempty" db:"result_line"`
	ConvertWorkloadID string `gorm:"column:convert_workload_id" json:"convertWorkloadId,omitempty" db:"convert_workload_id"`
	TemplateID        string `gorm:"column:template_id" json:"templateId,omitempty" db:"template_id"`
	TemplateVersion   int    `gorm:"column:template_version" json:"templateVersion,omitempty" db:"template_version"`
	ExperimentID      string `gorm:"column:experiment_id" json:"experimentId,omitempty" db:"experiment_id"`
	VariantName       string `gorm:"column:variant_name" json:"variantName,omitempty" db:"variant_name"`
	// Ext is JSON reserved for framework plugins; the platform does not interpret it.
	Ext string `gorm:"column:ext" json:"ext,omitempty" db:"ext"`

	StartTime    pq.NullTime `gorm:"column:start_time" json:"startTime,omitempty" db:"start_time"`
	EndTime      pq.NullTime `gorm:"column:end_time" json:"endTime,omitempty" db:"end_time"`
	CreatedAt    pq.NullTime `gorm:"column:created_at;autoCreateTime" json:"createdAt" db:"created_at"`
	UpdatedAt    pq.NullTime `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt" db:"updated_at"`
	DeletionTime pq.NullTime `gorm:"column:deletion_time" json:"deletionTime,omitempty" db:"deletion_time"`
	IsDeleted    bool        `gorm:"column:is_deleted" json:"isDeleted" db:"is_deleted"`
}

// TableName sets the SQL table name for PostTrainingRun.
func (PostTrainingRun) TableName() string {
	return TPostTrainingRun
}

// PostTrainingRunFilter selects runs for ListPostTrainingRuns. Zero-valued fields are ignored.
type PostTrainingRunFilter struct {
	Workspace    string
	Framework    string
	TrainType    string
	Method       string
	Statuses     []string
	Stage        string
	BaseModelID  string
	UserID       string
	TemplateID   string
	ExperimentID string
	Search       string // matches display_name ILIKE %search%
	Limit        int
	Offset       int
}

// CreatePostTrainingRun inserts a new run; run_id and workload_id must be unique.
func (c *Client) CreatePostTrainingRun(ctx context.Context, run *PostTrainingRun) error {
	if run == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Create(run).Error
}

// GetPostTrainingRun fetches a non-deleted run by run id.
func (c *Client) GetPostTrainingRun(ctx context.Context, runID string) (*PostTrainingRun, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var run PostTrainingRun
	if err := db.WithContext(ctx).
		Where("run_id = ? AND is_deleted = ?", runID, false).
		First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

// ListPostTrainingRuns returns the non-deleted runs matching filter, newest first, and their total count.
func (c *Client) ListPostTrainingRuns(
	ctx context.Context,
	filter PostTrainingRunFilter,
) ([]*PostTrainingRun, int64, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, 0, err
	}
	query := db.WithContext(ctx).Model(&PostTrainingRun{}).Where("is_deleted = ?", false)
	if filter.Workspace != "" {
		query = query.Where("workspace = ?", filter.Workspace)
	}
	if filter.Framework != "" {
		query = query.Where("framework = ?", filter.Framework)
	}
	if filter.TrainType != "" {
		query = query.Where("train_type = ?", filter.TrainType)
	}
	if filter.Method != "" {
		query = query.Where("method = ?", filter.Method)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	if filter.Stage != "" {
		query = query.Where("stage = ?", filter.Stage)
	}
	if filter.BaseModelID != "" {
		query = query.Where("base_model_id = ?", filter.BaseModelID)
	}
	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.TemplateID != "" {
		query = query.Where("template_id = ?", filter.TemplateID)
	}
	if filter.ExperimentID != "" {
		query = query.Where("experiment_id = ?", filter.ExperimentID)
	}
	if filter.Search != "" {
		query = query.Where("display_name ILIKE ?", "%"+filter.Search+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	var runs []*PostTrainingRun
	if err := query.Order("created_at DESC").Find(&runs).Error; err != nil {
		return nil, 0, err
	}
	return runs, total, nil
}

// UpdatePostTrainingRun sets the given columns on a non-deleted run and refreshes updated_at. It returns
// gorm.ErrRecordNotFound when no such run exists.
func (c *Client) UpdatePostTrainingRun(ctx context.Context, runID string, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return updatePostTrainingRun(db.WithContext(ctx), runID, fields)
}

// SetPostTrainingRunDeleted soft-deletes a run.
func (c *Client) SetPostTrainingRunDeleted(ctx context.Context, runID string) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).
		Model(&PostTrainingRun{}).
		Where("run_id = ?", runID).
		Updates(map[string]interface{}{
			"is_deleted":    true,
			"deletion_time": time.Now(),
		}).Error
}

// updatePostTrainingRun applies fields to the non-deleted run on db, which may be a transaction.
func updatePostTrainingRun(db *gorm.DB, runID string, fields map[string]interface{}) error {
	result := db.Model(&PostTrainingRun{}).
		Where("run_id = ? AND is_deleted = ?", runID, false).
		Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
