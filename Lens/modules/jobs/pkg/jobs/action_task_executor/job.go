// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package action_task_executor

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
)

// ActionTaskExecutorJob wraps ActionTaskExecutor as a standard Job
type ActionTaskExecutorJob struct {
	executor *ActionTaskExecutor
}

// NewActionTaskExecutorJob creates a new ActionTaskExecutorJob
func NewActionTaskExecutorJob() *ActionTaskExecutorJob {
	executor := NewActionTaskExecutor()

	RegisterDefaultHandlers(executor)

	log.Infof("Created ActionTaskExecutorJob (poll interval: %v)", executor.pollInterval)

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
