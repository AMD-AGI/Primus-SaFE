// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// WorkloadResourceFacadeInterface defines the interface for workload resource operations
type WorkloadResourceFacadeInterface interface {
	GetByWorkloadUID(ctx context.Context, workloadUID string) (*model.WorkloadResource, error)
	WithCluster(clusterName string) WorkloadResourceFacadeInterface
}

// WorkloadResourceFacade implements WorkloadResourceFacadeInterface
type WorkloadResourceFacade struct {
	BaseFacade
}

// NewWorkloadResourceFacade creates a new WorkloadResourceFacade
func NewWorkloadResourceFacade() *WorkloadResourceFacade {
	return &WorkloadResourceFacade{}
}

// WithCluster returns a new facade scoped to the given cluster
func (f *WorkloadResourceFacade) WithCluster(clusterName string) WorkloadResourceFacadeInterface {
	return &WorkloadResourceFacade{BaseFacade: f.withCluster(clusterName)}
}

// GetByWorkloadUID returns the workload resource record for the given workload UID
func (f *WorkloadResourceFacade) GetByWorkloadUID(ctx context.Context, workloadUID string) (*model.WorkloadResource, error) {
	d := f.getDAL()
	if d == nil {
		return nil, nil
	}
	t := d.WorkloadResource
	return t.WithContext(ctx).Where(t.WorkloadUID.Eq(workloadUID)).First()
}
