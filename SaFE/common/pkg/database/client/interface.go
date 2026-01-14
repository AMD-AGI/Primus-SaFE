/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
	NotificationInterface
	WorkloadStatisticInterface
	NodeStatisticInterface
	UserTokenInterface
	CDInterface
	PlaygroundSessionInterface
	ModelInterface
	ApiKeyInterface
	DatasetInterface
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
	SelectFaults(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*Fault, error)
	CountFaults(ctx context.Context, query sqrl.Sqlizer) (int, error)
	GetFault(ctx context.Context, uid string) (*Fault, error)
	DeleteFault(ctx context.Context, uid string) error
}

type OpsJobInterface interface {
	UpsertJob(ctx context.Context, job *OpsJob) error
	SelectJobs(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*OpsJob, error)
	CountJobs(ctx context.Context, query sqrl.Sqlizer) (int, error)
	GetOpsJob(ctx context.Context, jobId string) (*OpsJob, error)
	SetOpsJobDeleted(ctx context.Context, opsJobId string) error
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

type NotificationInterface interface {
	SubmitNotification(ctx context.Context, data *model.Notification) error
	ListUnprocessedNotifications(ctx context.Context) ([]*model.Notification, error)
	UpdateNotification(ctx context.Context, data *model.Notification) error
}

type WorkloadStatisticInterface interface {
	GetWorkloadStatisticByID(ctx context.Context, id int32) (*model.WorkloadStatistic, error)
	GetWorkloadStatisticByWorkloadID(ctx context.Context, workloadID string) (*model.WorkloadStatistic, error)
	GetWorkloadStatisticsByWorkloadID(ctx context.Context, workloadID string) ([]*model.WorkloadStatistic, error)
	GetWorkloadStatisticByWorkloadUID(ctx context.Context, workloadUID string) (*model.WorkloadStatistic, error)
	GetWorkloadStatisticsByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.WorkloadStatistic, error)
	GetWorkloadStatisticsByClusterAndWorkspace(ctx context.Context, cluster, workspace string) ([]*model.WorkloadStatistic, error)
	GetWorkloadStatisticsByType(ctx context.Context, statisticType string) ([]*model.WorkloadStatistic, error)
	CreateWorkloadStatistic(ctx context.Context, stat *model.WorkloadStatistic) error
	UpsertWorkloadStatistic(ctx context.Context, stat *model.WorkloadStatistic) error
	UpdateWorkloadStatistic(ctx context.Context, stat *model.WorkloadStatistic) error
	DeleteWorkloadStatistic(ctx context.Context, id int32) error
	DeleteWorkloadStatisticsByWorkloadID(ctx context.Context, workloadID string) error
}

type NodeStatisticInterface interface {
	GetNodeStatisticByID(ctx context.Context, id int32) (*model.NodeStatistic, error)
	GetNodeStatisticByClusterAndNode(ctx context.Context, cluster, nodeName string) (*model.NodeStatistic, error)
	GetNodeStatisticsByCluster(ctx context.Context, cluster string) ([]*model.NodeStatistic, error)
	GetNodeStatisticsByNodeNames(ctx context.Context, cluster string, nodeNames []string) ([]*model.NodeStatistic, error)
	GetNodeGpuUtilizationMap(ctx context.Context, cluster string, nodeNames []string) (map[string]float64, error)
	CreateNodeStatistic(ctx context.Context, stat *model.NodeStatistic) error
	UpdateNodeStatistic(ctx context.Context, stat *model.NodeStatistic) error
	UpsertNodeStatistic(ctx context.Context, stat *model.NodeStatistic) error
	DeleteNodeStatistic(ctx context.Context, id int32) error
	DeleteNodeStatisticByClusterAndNode(ctx context.Context, cluster, nodeName string) error
	DeleteNodeStatisticsByCluster(ctx context.Context, cluster string) error
}

type UserTokenInterface interface {
	UpsertUserToken(ctx context.Context, userToken *UserToken) error
	SelectUserTokens(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*UserToken, error)
}

type PlaygroundSessionInterface interface {
	InsertPlaygroundSession(ctx context.Context, session *PlaygroundSession) error
	UpdatePlaygroundSession(ctx context.Context, session *PlaygroundSession) error
	SelectPlaygroundSessions(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*PlaygroundSession, error)
	GetPlaygroundSession(ctx context.Context, id int64) (*PlaygroundSession, error)
	CountPlaygroundSessions(ctx context.Context, query sqrl.Sqlizer) (int, error)
	SetPlaygroundSessionDeleted(ctx context.Context, id int64) error
}

type CDInterface interface {
	CreateDeploymentRequest(ctx context.Context, req *DeploymentRequest) (int64, error)
	GetDeploymentRequest(ctx context.Context, id int64) (*DeploymentRequest, error)
	ListDeploymentRequests(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*DeploymentRequest, error)
	CountDeploymentRequests(ctx context.Context, query sqrl.Sqlizer) (int, error)
	UpdateDeploymentRequest(ctx context.Context, req *DeploymentRequest) error

	CreateEnvironmentSnapshot(ctx context.Context, snapshot *EnvironmentSnapshot) (int64, error)
	GetEnvironmentSnapshot(ctx context.Context, id int64) (*EnvironmentSnapshot, error)
	GetEnvironmentSnapshotByRequestId(ctx context.Context, reqId int64) (*EnvironmentSnapshot, error)
	ListEnvironmentSnapshots(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*EnvironmentSnapshot, error)
}

// ApiKeyInterface defines the interface for API key database operations
type ApiKeyInterface interface {
	// InsertApiKey inserts a new API key record
	InsertApiKey(ctx context.Context, apiKey *ApiKey) error
	// SelectApiKeys retrieves API keys based on query conditions
	SelectApiKeys(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*ApiKey, error)
	// CountApiKeys counts API keys based on query conditions
	CountApiKeys(ctx context.Context, query sqrl.Sqlizer) (int, error)
	// GetApiKeyById retrieves an API key by its ID
	GetApiKeyById(ctx context.Context, id int64) (*ApiKey, error)
	// GetApiKeyByKey retrieves an API key by the key value
	GetApiKeyByKey(ctx context.Context, apiKey string) (*ApiKey, error)
	// SetApiKeyDeleted performs soft delete on an API key
	SetApiKeyDeleted(ctx context.Context, userId string, id int64) error
}

// ModelInterface defines database operations for Model entities
type ModelInterface interface {
	UpsertModel(ctx context.Context, m *Model) error
	GetModelByID(ctx context.Context, id string) (*Model, error)
	ListModels(ctx context.Context, accessMode string, workspace string, isDeleted bool) ([]*Model, error)
	DeleteModel(ctx context.Context, id string) error
}

type DatasetInterface interface {
	UpsertDataset(ctx context.Context, dataset *Dataset) error
	SelectDatasets(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*Dataset, error)
	GetDataset(ctx context.Context, datasetId string) (*Dataset, error)
	CountDatasets(ctx context.Context, query sqrl.Sqlizer) (int, error)
	SetDatasetDeleted(ctx context.Context, datasetId string) error
	UpdateDatasetStatus(ctx context.Context, datasetId, status, message string) error
	UpdateDatasetFileInfo(ctx context.Context, datasetId string, totalSize int64, fileCount int) error
	UpdateDatasetDownloadStatus(ctx context.Context, datasetId, downloadStatus string) error
	UpdateDatasetLocalPath(ctx context.Context, datasetId, workspace, status, message string) error
}
