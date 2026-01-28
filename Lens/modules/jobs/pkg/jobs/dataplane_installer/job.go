// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dataplane_installer

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
)

const (
	// JobSchedule defines when the job runs (every 10 seconds)
	JobSchedule = "@every 10s"
)

// DataplaneInstallerJob wraps DataplaneInstaller as a standard Job
type DataplaneInstallerJob struct {
	installer *DataplaneInstaller
}

// NewDataplaneInstallerJob creates a new DataplaneInstallerJob
func NewDataplaneInstallerJob() *DataplaneInstallerJob {
	return &DataplaneInstallerJob{
		installer: NewDataplaneInstaller(),
	}
}

// Schedule returns the cron schedule for this job
func (j *DataplaneInstallerJob) Schedule() string {
	return JobSchedule
}

// Run executes the job
func (j *DataplaneInstallerJob) Run(
	ctx context.Context,
	k8sClient *clientsets.K8SClientSet,
	storageClient *clientsets.StorageClientSet,
) (*common.ExecutionStats, error) {
	// Only run in control plane mode
	if !clientsets.IsControlPlaneMode() {
		return &common.ExecutionStats{}, nil
	}

	stats, err := j.installer.Run(ctx)
	if err != nil {
		log.Errorf("DataplaneInstallerJob failed: %v", err)
	}
	return stats, err
}
