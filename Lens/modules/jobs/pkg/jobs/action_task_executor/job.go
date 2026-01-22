// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package action_task_executor

import (
	"context"
	"os"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
)

// ActionTaskExecutorJob wraps ActionTaskExecutor as a standard Job
type ActionTaskExecutorJob struct {
	executor *ActionTaskExecutor
}

// NewActionTaskExecutorJob creates a new ActionTaskExecutorJob
// It automatically detects the current cluster name from environment
func NewActionTaskExecutorJob() *ActionTaskExecutorJob {
	// Get cluster name from environment or ClusterManager
	clusterName := getClusterName()

	executor := NewActionTaskExecutor(clusterName)

	// Register default handlers
	RegisterDefaultHandlers(executor)

	log.Infof("Created ActionTaskExecutorJob for cluster: %s (poll interval: %v)",
		clusterName, executor.pollInterval)

	return &ActionTaskExecutorJob{
		executor: executor,
	}
}

// Schedule returns the cron schedule for this job
// Using @every 300ms for <1s latency requirement
func (j *ActionTaskExecutorJob) Schedule() string {
	return JobSchedule
}

// Run executes the job
func (j *ActionTaskExecutorJob) Run(
	ctx context.Context,
	k8sClient *clientsets.K8SClientSet,
	storageClient *clientsets.StorageClientSet,
) (*common.ExecutionStats, error) {
	return j.executor.Run(ctx, k8sClient, storageClient)
}

// getClusterName returns the current cluster name
func getClusterName() string {
	// First try environment variable
	if name := os.Getenv("CLUSTER_NAME"); name != "" {
		return name
	}

	// Try to get from ClusterManager
	cm := clientsets.GetClusterManager()
	if cm != nil {
		return cm.GetCurrentClusterName()
	}

	// Default fallback
	return "default"
}
