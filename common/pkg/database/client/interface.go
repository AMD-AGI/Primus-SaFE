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
	PublicKeyInterface
	SshSessionRecordsInterface
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
	GetFault(ctx context.Context, uid string) (*Fault, error)
	DeleteFault(ctx context.Context, uid string) error
}

type OpsJobInterface interface {
	UpsertJob(ctx context.Context, job *OpsJob) error
	SelectJobs(ctx context.Context, query sqrl.Sqlizer, sortBy, order string, limit, offset int) ([]*OpsJob, error)
	CountJobs(ctx context.Context, query sqrl.Sqlizer) (int, error)
	SetOpsJobDeleted(ctx context.Context, opsJobId, userId string) error
}

type PublicKeyInterface interface {
	InsertPublicKey(ctx context.Context, publicKey *PublicKey) error
	SelectPublicKeys(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*PublicKey, error)
	CountPublicKeys(ctx context.Context, query sqrl.Sqlizer) (int, error)
	DeletePublicKey(ctx context.Context, userId string, id int64) error
	GetPublicKeyByUserId(ctx context.Context, userId string) ([]*PublicKey, error)
	SetPublicKeyStatus(ctx context.Context, userId string, id int64, status bool) error
	SetPublicKeyDescription(ctx context.Context, userId string, id int64, description string) error
}

type SshSessionRecordsInterface interface {
	InsertSshSessionRecord(ctx context.Context, record *SshSessionRecords) (int64, error)
	SetSshDisconnect(ctx context.Context, id int64, disconnectReason string) error
}
