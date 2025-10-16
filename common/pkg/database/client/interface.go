/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"

	sqrl "github.com/Masterminds/squirrel"
)

type Interface interface {
	WorkloadInterface
	FaultInterface
	OpsJobInterface
	ImageInterface
	ImageDigestInterface
	ImageImportJobInterface
	RegistryInfoInterface
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

type ImageInterface interface {
	UpsertImage(ctx context.Context, image *model.Image) error
	SelectImages(ctx context.Context, filter *ImageFilter) ([]*model.Image, int, error)
	GetImage(ctx context.Context, imageId int32) (*model.Image, error)
	GetImageByTag(ctx context.Context, tag string) (*model.Image, error)
	DeleteImage(ctx context.Context, id int32, deletedBy string) error
}

type ImageDigestInterface interface {
	UpsertImageDigest(ctx context.Context, digest *model.ImageDigest) error
	DeleteImageDigest(ctx context.Context, id int32) error
}

type ImageImportJobInterface interface {
	GetImageImportJobByJobName(ctx context.Context, jobName string) (*model.ImageImportJob, error)
	GetImageImportJobByTag(ctx context.Context, tag string) (*model.ImageImportJob, error)
	UpsertImageImportJob(ctx context.Context, job *model.ImageImportJob) error
	GetImportImageByImageID(ctx context.Context, imageID int32) (*model.ImageImportJob, error)
	UpdateImageImportJob(ctx context.Context, job *model.ImageImportJob) error
}

type RegistryInfoInterface interface {
	UpsertRegistryInfo(ctx context.Context, registryInfo *model.RegistryInfo) error
	GetDefaultRegistryInfo(ctx context.Context) (*model.RegistryInfo, error)
	GetRegistryInfoByUrl(ctx context.Context, url string) (*model.RegistryInfo, error)
	GetRegistryInfoById(ctx context.Context, id int32) (*model.RegistryInfo, error)
	DeleteRegistryInfo(ctx context.Context, id int32) error
	ListRegistryInfos(ctx context.Context, pageNum, pageSize int) ([]*model.RegistryInfo, error)
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
