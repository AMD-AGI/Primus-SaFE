// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
)

// ActionTask status constants (duplicated from database package to avoid circular import)
const (
	actionTaskStatusPending   = "pending"
	actionTaskStatusRunning   = "running"
	actionTaskStatusCompleted = "completed"
	actionTaskStatusFailed    = "failed"
	actionTaskStatusTimeout   = "timeout"
)

// ActionRequest represents a request to execute an action on a remote cluster
type ActionRequest struct {
	Type       string                 `json:"type"`        // Action type: get_process_tree, pyspy_sample, etc.
	TargetType string                 `json:"target_type"` // Target resource type: pod, node, process
	TargetID   string                 `json:"target_id"`   // Target resource ID: Pod UID, Node Name, etc.
	TargetNode string                 `json:"target_node"` // Target node name
	Parameters map[string]interface{} `json:"parameters"`  // Action parameters
	Timeout    time.Duration          `json:"timeout"`     // Timeout duration (0 means use default)
}

// ActionResult represents the result of an action execution
type ActionResult struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// ActionClient encapsulates action task creation and synchronous waiting logic
// It provides a synchronous API experience while using asynchronous task queue underneath
type ActionClient struct {
	db             *gorm.DB
	clusterName    string
	pollInterval   time.Duration
	defaultTimeout time.Duration
}

// NewActionClient creates a new ActionClient for the specified cluster
func NewActionClient(db *gorm.DB, clusterName string) *ActionClient {
	return &ActionClient{
		db:             db,
		clusterName:    clusterName,
		pollInterval:   200 * time.Millisecond,
		defaultTimeout: 30 * time.Second,
	}
}

// NewActionClientFromClusterSet creates a new ActionClient from a ClusterClientSet
func NewActionClientFromClusterSet(clientSet *ClusterClientSet) (*ActionClient, error) {
	if clientSet == nil {
		return nil, fmt.Errorf("client set is nil")
	}
	if clientSet.StorageClientSet == nil || clientSet.StorageClientSet.DB == nil {
		return nil, fmt.Errorf("storage client or DB is nil for cluster %s", clientSet.ClusterName)
	}
	return NewActionClient(clientSet.StorageClientSet.DB, clientSet.ClusterName), nil
}

// WithPollInterval sets the polling interval for waiting on task completion
func (c *ActionClient) WithPollInterval(interval time.Duration) *ActionClient {
	c.pollInterval = interval
	return c
}

// WithDefaultTimeout sets the default timeout for action execution
func (c *ActionClient) WithDefaultTimeout(timeout time.Duration) *ActionClient {
	c.defaultTimeout = timeout
	return c
}

// ExecuteAction creates an action task and waits synchronously for its completion
// This method is transparent to the caller - they experience it as a synchronous call
func (c *ActionClient) ExecuteAction(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	timeout := c.defaultTimeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	// 1. Create the task
	task, err := c.createTask(ctx, req, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create action task: %w", err)
	}

	log.Debugf("Created action task %d for cluster %s: type=%s, target=%s/%s",
		task.ID, c.clusterName, req.Type, req.TargetType, req.TargetID)

	// 2. Wait for completion (polling)
	result, err := c.waitForCompletion(ctx, task.ID, timeout)
	if err != nil {
		// Mark task as timeout if context is canceled or timed out
		_ = c.markTaskTimeout(ctx, task.ID, err.Error())
		return nil, err
	}

	return result, nil
}

// createTask creates a new action task in the database
func (c *ActionClient) createTask(ctx context.Context, req *ActionRequest, timeout time.Duration) (*model.ActionTasks, error) {
	// Serialize parameters
	var params model.ExtType
	if req.Parameters != nil {
		params = model.ExtType(req.Parameters)
	} else {
		params = make(model.ExtType)
	}

	task := &model.ActionTasks{
		ClusterName:    c.clusterName,
		ActionType:     req.Type,
		TargetType:     req.TargetType,
		TargetID:       req.TargetID,
		TargetNode:     req.TargetNode,
		Parameters:     params,
		Status:         actionTaskStatusPending,
		TimeoutSeconds: int32(timeout.Seconds()),
		CreatedAt:      time.Now(),
	}

	if err := c.db.WithContext(ctx).Create(task).Error; err != nil {
		return nil, err
	}

	return task, nil
}

// waitForCompletion polls the database until the task is completed, failed, or timed out
func (c *ActionClient) waitForCompletion(ctx context.Context, taskID int64, timeout time.Duration) (*ActionResult, error) {
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	timeoutCh := time.After(timeout)

	for {
		select {
		case <-ticker.C:
			task, err := c.getTask(ctx, taskID)
			if err != nil {
				log.Warnf("Failed to query action task %d: %v", taskID, err)
				continue
			}

			switch task.Status {
			case actionTaskStatusCompleted:
				// Success - parse result
				var data json.RawMessage
				if task.Result != nil {
					resultBytes, err := json.Marshal(task.Result)
					if err != nil {
						return nil, fmt.Errorf("failed to marshal result: %w", err)
					}
					data = resultBytes
				}
				return &ActionResult{Success: true, Data: data}, nil

			case actionTaskStatusFailed:
				return &ActionResult{Success: false, Error: task.ErrorMessage}, nil

			case actionTaskStatusTimeout:
				return nil, fmt.Errorf("task timed out on executor side: %s", task.ErrorMessage)

			// pending or running: continue waiting
			}

		case <-timeoutCh:
			return nil, fmt.Errorf("action timeout after %v waiting for task %d", timeout, taskID)

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// getTask retrieves an action task by ID
func (c *ActionClient) getTask(ctx context.Context, taskID int64) (*model.ActionTasks, error) {
	var task model.ActionTasks
	if err := c.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// markTaskTimeout marks a task as timed out
func (c *ActionClient) markTaskTimeout(ctx context.Context, taskID int64, errorMsg string) error {
	now := time.Now()
	return c.db.WithContext(ctx).Model(&model.ActionTasks{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        actionTaskStatusTimeout,
			"error_message": errorMsg,
			"completed_at":  now,
		}).Error
}

// GetActionClientForCluster creates an ActionClient for the specified cluster
// This is a convenience method that works with the ClusterManager
func GetActionClientForCluster(clusterName string) (*ActionClient, error) {
	cm := GetClusterManager()
	clientSet, err := cm.GetClientSetByClusterName(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client set for cluster %s: %w", clusterName, err)
	}
	return NewActionClientFromClusterSet(clientSet)
}

// IsRemoteCluster checks if the specified cluster is different from the current cluster
func IsRemoteCluster(clusterName string) bool {
	cm := GetClusterManager()
	currentCluster := cm.GetCurrentClusterName()
	return clusterName != "" && clusterName != currentCluster
}
