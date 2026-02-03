// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// AnalysisTaskFacade provides database operations for AI analysis tasks
type AnalysisTaskFacade struct {
	db          *gorm.DB
	clusterName string
}

// AnalysisTask represents an analysis task with parsed ext fields
type AnalysisTask struct {
	ID           int64                  `json:"id"`
	Type         string                 `json:"type"`
	TypeDisplay  string                 `json:"type_display"`
	Status       string                 `json:"status"`
	StatusDisplay string                `json:"status_display"`
	CreatedAt    time.Time              `json:"created_at"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	DurationMs   *int64                 `json:"duration_ms,omitempty"`
	RunID        int64                  `json:"run_id"`
	GithubRunID  int64                  `json:"github_run_id"`
	CommitSHA    string                 `json:"commit_sha,omitempty"`
	RepoName     string                 `json:"repo_name,omitempty"`
	WorkflowName string                 `json:"workflow_name,omitempty"`
	Branch       string                 `json:"branch,omitempty"`
	Result       *AnalysisResult        `json:"result,omitempty"`
	Error        *AnalysisError         `json:"error,omitempty"`
}

// AnalysisResult represents the result of an analysis task
type AnalysisResult struct {
	Summary       string           `json:"summary"`
	RiskLevel     string           `json:"risk_level,omitempty"`
	FindingsCount int              `json:"findings_count,omitempty"`
	Categories    []string         `json:"categories,omitempty"`
	ReportURL     string           `json:"report_url,omitempty"`
	Details       []AnalysisFinding `json:"details,omitempty"`
}

// AnalysisFinding represents a single finding in the analysis
type AnalysisFinding struct {
	File   string `json:"file"`
	Risk   string `json:"risk"`
	Reason string `json:"reason"`
}

// AnalysisError represents an error that occurred during analysis
type AnalysisError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	RetryCount int    `json:"retry_count,omitempty"`
}

// AnalysisTaskSummary provides summary statistics for analysis tasks
type AnalysisTaskSummary struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

// NewAnalysisTaskFacade creates a new analysis task facade
func NewAnalysisTaskFacade() *AnalysisTaskFacade {
	return &AnalysisTaskFacade{
		clusterName: "",
	}
}

// NewAnalysisTaskFacadeForCluster creates a new analysis task facade for a specific cluster
func NewAnalysisTaskFacadeForCluster(clusterName string) *AnalysisTaskFacade {
	return &AnalysisTaskFacade{
		clusterName: clusterName,
	}
}

// getDB returns the database connection
func (f *AnalysisTaskFacade) getDB() *gorm.DB {
	if f.db == nil {
		if f.clusterName == "" {
			f.db = GetFacade().GetSystemConfig().GetDB()
		} else {
			f.db = GetFacadeForCluster(f.clusterName).GetSystemConfig().GetDB()
		}
	}
	return f.db
}

// GetTasksByRunID retrieves all analysis tasks for a specific workflow run
func (f *AnalysisTaskFacade) GetTasksByRunID(ctx context.Context, runID int64) ([]*AnalysisTask, error) {
	var tasks []*model.WorkloadTaskState
	
	// Query tasks with run_id in ext field and analysis task types
	err := f.getDB().WithContext(ctx).
		Where("task_type IN ?", constant.AnalysisTaskTypes).
		Where("(ext->>'run_id')::bigint = ?", runID).
		Order("created_at ASC").
		Find(&tasks).Error
	
	if err != nil {
		return nil, err
	}
	
	return f.convertTasks(tasks), nil
}

// GetTasksByGithubRunID retrieves all analysis tasks for a specific GitHub run ID
func (f *AnalysisTaskFacade) GetTasksByGithubRunID(ctx context.Context, githubRunID int64) ([]*AnalysisTask, error) {
	var tasks []*model.WorkloadTaskState
	
	err := f.getDB().WithContext(ctx).
		Where("task_type IN ?", constant.AnalysisTaskTypes).
		Where("(ext->>'github_run_id')::bigint = ?", githubRunID).
		Order("created_at ASC").
		Find(&tasks).Error
	
	if err != nil {
		return nil, err
	}
	
	return f.convertTasks(tasks), nil
}

// GetTaskByID retrieves a single analysis task by ID
func (f *AnalysisTaskFacade) GetTaskByID(ctx context.Context, taskID int64) (*AnalysisTask, error) {
	var task model.WorkloadTaskState
	
	err := f.getDB().WithContext(ctx).
		Where("id = ?", taskID).
		Where("task_type IN ?", constant.AnalysisTaskTypes).
		First(&task).Error
	
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	
	return f.convertTask(&task), nil
}

// ListTasks retrieves analysis tasks with optional filters
func (f *AnalysisTaskFacade) ListTasks(ctx context.Context, opts ListAnalysisTasksOptions) ([]*AnalysisTask, int64, error) {
	var tasks []*model.WorkloadTaskState
	var total int64
	
	query := f.getDB().WithContext(ctx).Model(&model.WorkloadTaskState{}).
		Where("task_type IN ?", constant.AnalysisTaskTypes)
	
	// Apply filters
	if opts.TaskType != "" {
		query = query.Where("task_type = ?", opts.TaskType)
	}
	if opts.Status != "" {
		query = query.Where("status = ?", opts.Status)
	}
	if opts.RepoName != "" {
		query = query.Where("ext->>'repo_name' = ?", opts.RepoName)
	}
	if !opts.StartTime.IsZero() {
		query = query.Where("created_at >= ?", opts.StartTime)
	}
	if !opts.EndTime.IsZero() {
		query = query.Where("created_at <= ?", opts.EndTime)
	}
	
	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	
	// Apply pagination
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}
	
	// Execute query
	if err := query.Order("created_at DESC").Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	
	return f.convertTasks(tasks), total, nil
}

// ListAnalysisTasksOptions defines options for listing analysis tasks
type ListAnalysisTasksOptions struct {
	TaskType  string
	Status    string
	RepoName  string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	Offset    int
}

// RetryTask resets a failed task to pending status for retry
func (f *AnalysisTaskFacade) RetryTask(ctx context.Context, taskID int64) error {
	result := f.getDB().WithContext(ctx).
		Model(&model.WorkloadTaskState{}).
		Where("id = ?", taskID).
		Where("task_type IN ?", constant.AnalysisTaskTypes).
		Where("status = ?", constant.TaskStatusFailed).
		Updates(map[string]interface{}{
			"status":     constant.TaskStatusPending,
			"updated_at": time.Now(),
			"ext":        gorm.Expr("ext - 'error' || jsonb_build_object('retry_count', COALESCE((ext->>'retry_count')::int, 0) + 1)"),
		})
	
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("task not found or not in failed status")
	}
	
	return nil
}

// UpdateTask updates task status and merges ext fields
func (f *AnalysisTaskFacade) UpdateTask(ctx context.Context, taskID int64, status string, extUpdates map[string]interface{}) error {
	// Validate status
	validStatuses := map[string]bool{
		constant.TaskStatusPending:   true,
		constant.TaskStatusRunning:   true,
		constant.TaskStatusCompleted: true,
		constant.TaskStatusFailed:    true,
		constant.TaskStatusCancelled: true,
	}
	if status != "" && !validStatuses[status] {
		return fmt.Errorf("invalid status: %s", status)
	}
	
	// Build updates
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	
	if status != "" {
		updates["status"] = status
	}
	
	// Build the query
	query := f.getDB().WithContext(ctx).
		Model(&model.WorkloadTaskState{}).
		Where("id = ?", taskID).
		Where("task_type IN ?", constant.AnalysisTaskTypes)
	
	// If we have ext updates, merge them into existing ext
	if len(extUpdates) > 0 {
		// Convert extUpdates to JSON for JSONB merge
		extJSON, err := json.Marshal(extUpdates)
		if err != nil {
			return fmt.Errorf("failed to marshal ext updates: %w", err)
		}
		updates["ext"] = gorm.Expr("COALESCE(ext, '{}'::jsonb) || ?::jsonb", string(extJSON))
	}
	
	result := query.Updates(updates)
	
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("task not found")
	}
	
	return nil
}

// GetSummaryByRunID returns summary statistics for analysis tasks of a specific run
func (f *AnalysisTaskFacade) GetSummaryByRunID(ctx context.Context, runID int64) (*AnalysisTaskSummary, error) {
	type statusCount struct {
		Status string
		Count  int
	}
	
	var counts []statusCount
	err := f.getDB().WithContext(ctx).
		Model(&model.WorkloadTaskState{}).
		Select("status, COUNT(*) as count").
		Where("task_type IN ?", constant.AnalysisTaskTypes).
		Where("(ext->>'run_id')::bigint = ?", runID).
		Group("status").
		Find(&counts).Error
	
	if err != nil {
		return nil, err
	}
	
	summary := &AnalysisTaskSummary{}
	for _, c := range counts {
		summary.Total += c.Count
		switch c.Status {
		case constant.TaskStatusPending:
			summary.Pending = c.Count
		case constant.TaskStatusRunning:
			summary.Running = c.Count
		case constant.TaskStatusCompleted:
			summary.Completed = c.Count
		case constant.TaskStatusFailed:
			summary.Failed = c.Count
		}
	}
	
	return summary, nil
}

// convertTasks converts a slice of WorkloadTaskState to AnalysisTask
func (f *AnalysisTaskFacade) convertTasks(tasks []*model.WorkloadTaskState) []*AnalysisTask {
	result := make([]*AnalysisTask, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, f.convertTask(t))
	}
	return result
}

// convertTask converts a WorkloadTaskState to AnalysisTask
func (f *AnalysisTaskFacade) convertTask(task *model.WorkloadTaskState) *AnalysisTask {
	at := &AnalysisTask{
		ID:           task.ID,
		Type:         task.TaskType,
		TypeDisplay:  constant.AnalysisTaskTypeDisplayNames[task.TaskType],
		Status:       task.Status,
		StatusDisplay: getStatusDisplay(task.Status),
		CreatedAt:    task.CreatedAt,
	}
	
	// Parse ext fields
	if task.Ext != nil {
		// Basic info
		if v, ok := task.Ext["run_id"].(float64); ok {
			at.RunID = int64(v)
		}
		if v, ok := task.Ext["github_run_id"].(float64); ok {
			at.GithubRunID = int64(v)
		}
		if v, ok := task.Ext["commit_sha"].(string); ok {
			at.CommitSHA = v
		}
		if v, ok := task.Ext["repo_name"].(string); ok {
			at.RepoName = v
		}
		if v, ok := task.Ext["workflow_name"].(string); ok {
			at.WorkflowName = v
		}
		if v, ok := task.Ext["branch"].(string); ok {
			at.Branch = v
		}
		
		// Timing info
		if v, ok := task.Ext["started_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				at.StartedAt = &t
			}
		}
		if v, ok := task.Ext["completed_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				at.CompletedAt = &t
			}
		}
		
		// Calculate duration if we have both started and completed times
		if at.StartedAt != nil && at.CompletedAt != nil {
			duration := at.CompletedAt.Sub(*at.StartedAt).Milliseconds()
			at.DurationMs = &duration
		} else if at.StartedAt != nil && task.Status == constant.TaskStatusRunning {
			// For running tasks, calculate duration from started to now
			duration := time.Since(*at.StartedAt).Milliseconds()
			at.DurationMs = &duration
		}
		
		// Result
		if resultMap, ok := task.Ext["result"].(map[string]interface{}); ok {
			at.Result = parseAnalysisResult(resultMap)
		}
		
		// Error
		if errorMap, ok := task.Ext["error"].(map[string]interface{}); ok {
			at.Error = parseAnalysisError(errorMap)
		}
	}
	
	return at
}

// getStatusDisplay returns the display name for a status
func getStatusDisplay(status string) string {
	switch status {
	case constant.TaskStatusPending:
		return "Pending"
	case constant.TaskStatusRunning:
		return "Running"
	case constant.TaskStatusCompleted:
		return "Completed"
	case constant.TaskStatusFailed:
		return "Failed"
	case constant.TaskStatusCancelled:
		return "Cancelled"
	default:
		return status
	}
}

// parseAnalysisResult parses the result map from ext field
func parseAnalysisResult(m map[string]interface{}) *AnalysisResult {
	result := &AnalysisResult{}
	
	if v, ok := m["summary"].(string); ok {
		result.Summary = v
	}
	if v, ok := m["risk_level"].(string); ok {
		result.RiskLevel = v
	}
	if v, ok := m["findings_count"].(float64); ok {
		result.FindingsCount = int(v)
	}
	if v, ok := m["categories"].([]interface{}); ok {
		for _, c := range v {
			if s, ok := c.(string); ok {
				result.Categories = append(result.Categories, s)
			}
		}
	}
	if v, ok := m["report_url"].(string); ok {
		result.ReportURL = v
	}
	if v, ok := m["details"].([]interface{}); ok {
		for _, d := range v {
			if dm, ok := d.(map[string]interface{}); ok {
				finding := AnalysisFinding{}
				if f, ok := dm["file"].(string); ok {
					finding.File = f
				}
				if r, ok := dm["risk"].(string); ok {
					finding.Risk = r
				}
				if reason, ok := dm["reason"].(string); ok {
					finding.Reason = reason
				}
				result.Details = append(result.Details, finding)
			}
		}
	}
	
	return result
}

// parseAnalysisError parses the error map from ext field
func parseAnalysisError(m map[string]interface{}) *AnalysisError {
	err := &AnalysisError{}
	
	if v, ok := m["code"].(string); ok {
		err.Code = v
	}
	if v, ok := m["message"].(string); ok {
		err.Message = v
	}
	if v, ok := m["retry_count"].(float64); ok {
		err.RetryCount = int(v)
	}
	
	return err
}
