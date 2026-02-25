// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// ProfilerFileFacadeInterface defines the interface for profiler file operations
type ProfilerFileFacadeInterface interface {
	ListByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.ProfilerFiles, error)
	WithCluster(clusterName string) ProfilerFileFacadeInterface
}

// ProfilerFileFacade implements ProfilerFileFacadeInterface
type ProfilerFileFacade struct {
	BaseFacade
}

// NewProfilerFileFacade creates a new ProfilerFileFacade
func NewProfilerFileFacade() *ProfilerFileFacade {
	return &ProfilerFileFacade{}
}

// WithCluster returns a new facade scoped to the given cluster
func (f *ProfilerFileFacade) WithCluster(clusterName string) ProfilerFileFacadeInterface {
	return &ProfilerFileFacade{BaseFacade: f.withCluster(clusterName)}
}

// ListByWorkloadUID returns all profiler files for the given workload UID
func (f *ProfilerFileFacade) ListByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.ProfilerFiles, error) {
	d := f.getDAL()
	if d == nil {
		return nil, nil
	}
	t := d.ProfilerFiles
	return t.WithContext(ctx).Where(t.WorkloadUID.Eq(workloadUID)).Order(t.DetectedAt.Desc()).Find()
}
