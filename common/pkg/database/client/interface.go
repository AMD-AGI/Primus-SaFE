/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"

	sqrl "github.com/Masterminds/squirrel"
)

type Interface interface {
	WorkloadInterface
	FaultInterface
	OpsJobInterface
}

type WorkloadInterface interface {
	UpsertWorkload(ctx context.Context, workload *Workload) error
	SelectWorkloads(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*Workload, error)
	GetWorkload(ctx context.Context, workloadId string) (*Workload, error)
	CountWorkloads(ctx context.Context, query sqrl.Sqlizer) (int, error)
	SetWorkloadDeleted(ctx context.Context, workloadId string) error
	SetWorkloadStopped(ctx context.Context, workloadId string) error
	SetWorkloadDescription(ctx context.Context, workloadId, description string) error
}

type FaultInterface interface {
	UpsertFault(ctx context.Context, fault *Fault) error
	SelectFaults(ctx context.Context, query sqrl.Sqlizer, sortBy, order string, limit, offset int) ([]*Fault, error)
	CountFaults(ctx context.Context, query sqrl.Sqlizer) (int, error)
}

type OpsJobInterface interface {
	UpsertJob(ctx context.Context, job *OpsJob) error
	SelectJobs(ctx context.Context, query sqrl.Sqlizer, sortBy, order string, limit, offset int) ([]*OpsJob, error)
	CountJobs(ctx context.Context, query sqrl.Sqlizer) (int, error)
	SetOpsJobDeleted(ctx context.Context, opsJobId, userId string) error
}
