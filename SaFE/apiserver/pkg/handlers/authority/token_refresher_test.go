/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"testing"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

// MockDBClient is a mock implementation of dbclient.Interface for testing
type MockDBClient struct {
	mock.Mock
}

// Implement UserTokenInterface
func (m *MockDBClient) UpsertUserToken(ctx context.Context, userToken *dbclient.UserToken) error {
	args := m.Called(ctx, userToken)
	return args.Error(0)
}

func (m *MockDBClient) SelectUserTokens(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*dbclient.UserToken, error) {
	args := m.Called(ctx, query, orderBy, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dbclient.UserToken), args.Error(1)
}

// Implement other interfaces with empty methods (required by dbclient.Interface)
func (m *MockDBClient) UpsertWorkload(ctx context.Context, workload *dbclient.Workload) error {
	return nil
}
func (m *MockDBClient) SelectWorkloads(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*dbclient.Workload, error) {
	return nil, nil
}
func (m *MockDBClient) GetWorkload(ctx context.Context, workloadId string) (*dbclient.Workload, error) {
	return nil, nil
}
func (m *MockDBClient) CountWorkloads(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	return 0, nil
}
func (m *MockDBClient) SetWorkloadDeleted(ctx context.Context, workloadId string) error { return nil }
func (m *MockDBClient) SetWorkloadStopped(ctx context.Context, workloadId string) error { return nil }
func (m *MockDBClient) SetWorkloadDescription(ctx context.Context, workloadId, description string) error {
	return nil
}

func (m *MockDBClient) UpsertFault(ctx context.Context, fault *dbclient.Fault) error { return nil }
func (m *MockDBClient) SelectFaults(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*dbclient.Fault, error) {
	return nil, nil
}
func (m *MockDBClient) CountFaults(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	return 0, nil
}
func (m *MockDBClient) GetFault(ctx context.Context, uid string) (*dbclient.Fault, error) {
	return nil, nil
}
func (m *MockDBClient) DeleteFault(ctx context.Context, uid string) error { return nil }

func (m *MockDBClient) UpsertJob(ctx context.Context, job *dbclient.OpsJob) error { return nil }
func (m *MockDBClient) SelectJobs(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*dbclient.OpsJob, error) {
	return nil, nil
}
func (m *MockDBClient) CountJobs(ctx context.Context, query sqrl.Sqlizer) (int, error) { return 0, nil }
func (m *MockDBClient) SetOpsJobDeleted(ctx context.Context, opsJobId string) error    { return nil }

func (m *MockDBClient) UpsertImage(ctx context.Context, image *model.Image) error { return nil }
func (m *MockDBClient) SelectImages(ctx context.Context, filter *dbclient.ImageFilter) ([]*model.Image, int, error) {
	return nil, 0, nil
}
func (m *MockDBClient) GetImage(ctx context.Context, imageId int32) (*model.Image, error) {
	return nil, nil
}
func (m *MockDBClient) GetImageByTag(ctx context.Context, tag string) (*model.Image, error) {
	return nil, nil
}
func (m *MockDBClient) DeleteImage(ctx context.Context, id int32, deletedBy string) error { return nil }

func (m *MockDBClient) UpsertImageDigest(ctx context.Context, digest *model.ImageDigest) error {
	return nil
}
func (m *MockDBClient) DeleteImageDigest(ctx context.Context, id int32) error { return nil }

func (m *MockDBClient) GetImageImportJobByJobName(ctx context.Context, jobName string) (*model.ImageImportJob, error) {
	return nil, nil
}
func (m *MockDBClient) GetImageImportJobByTag(ctx context.Context, tag string) (*model.ImageImportJob, error) {
	return nil, nil
}
func (m *MockDBClient) UpsertImageImportJob(ctx context.Context, job *model.ImageImportJob) error {
	return nil
}
func (m *MockDBClient) GetImportImageByImageID(ctx context.Context, imageID int32) (*model.ImageImportJob, error) {
	return nil, nil
}
func (m *MockDBClient) UpdateImageImportJob(ctx context.Context, job *model.ImageImportJob) error {
	return nil
}

func (m *MockDBClient) UpsertRegistryInfo(ctx context.Context, registryInfo *model.RegistryInfo) error {
	return nil
}
func (m *MockDBClient) GetDefaultRegistryInfo(ctx context.Context) (*model.RegistryInfo, error) {
	return nil, nil
}
func (m *MockDBClient) GetRegistryInfoByUrl(ctx context.Context, url string) (*model.RegistryInfo, error) {
	return nil, nil
}
func (m *MockDBClient) GetRegistryInfoById(ctx context.Context, id int32) (*model.RegistryInfo, error) {
	return nil, nil
}
func (m *MockDBClient) DeleteRegistryInfo(ctx context.Context, id int32) error { return nil }
func (m *MockDBClient) ListRegistryInfos(ctx context.Context, pageNum, pageSize int) ([]*model.RegistryInfo, error) {
	return nil, nil
}

func (m *MockDBClient) InsertPublicKey(ctx context.Context, publicKey *dbclient.PublicKey) error {
	return nil
}
func (m *MockDBClient) SelectPublicKeys(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*dbclient.PublicKey, error) {
	return nil, nil
}
func (m *MockDBClient) CountPublicKeys(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	return 0, nil
}
func (m *MockDBClient) DeletePublicKey(ctx context.Context, userId string, id int64) error {
	return nil
}
func (m *MockDBClient) GetPublicKeyByUserId(ctx context.Context, userId string) ([]*dbclient.PublicKey, error) {
	return nil, nil
}
func (m *MockDBClient) SetPublicKeyStatus(ctx context.Context, userId string, id int64, status bool) error {
	return nil
}
func (m *MockDBClient) SetPublicKeyDescription(ctx context.Context, userId string, id int64, description string) error {
	return nil
}

func (m *MockDBClient) InsertSshSessionRecord(ctx context.Context, record *dbclient.SshSessionRecords) (int64, error) {
	return 0, nil
}
func (m *MockDBClient) SetSshDisconnect(ctx context.Context, id int64, disconnectReason string) error {
	return nil
}

func (m *MockDBClient) SubmitNotification(ctx context.Context, data *model.Notification) error {
	return nil
}
func (m *MockDBClient) ListUnprocessedNotifications(ctx context.Context) ([]*model.Notification, error) {
	return nil, nil
}
func (m *MockDBClient) UpdateNotification(ctx context.Context, data *model.Notification) error {
	return nil
}

func (m *MockDBClient) GetWorkloadStatisticByID(ctx context.Context, id int32) (*model.WorkloadStatistic, error) {
	return nil, nil
}
func (m *MockDBClient) GetWorkloadStatisticByWorkloadID(ctx context.Context, workloadID string) (*model.WorkloadStatistic, error) {
	return nil, nil
}
func (m *MockDBClient) GetWorkloadStatisticsByWorkloadID(ctx context.Context, workloadID string) ([]*model.WorkloadStatistic, error) {
	return nil, nil
}
func (m *MockDBClient) GetWorkloadStatisticByWorkloadUID(ctx context.Context, workloadUID string) (*model.WorkloadStatistic, error) {
	return nil, nil
}
func (m *MockDBClient) GetWorkloadStatisticsByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.WorkloadStatistic, error) {
	return nil, nil
}
func (m *MockDBClient) GetWorkloadStatisticsByClusterAndWorkspace(ctx context.Context, cluster, workspace string) ([]*model.WorkloadStatistic, error) {
	return nil, nil
}
func (m *MockDBClient) GetWorkloadStatisticsByType(ctx context.Context, statisticType string) ([]*model.WorkloadStatistic, error) {
	return nil, nil
}
func (m *MockDBClient) CreateWorkloadStatistic(ctx context.Context, stat *model.WorkloadStatistic) error {
	return nil
}
func (m *MockDBClient) UpsertWorkloadStatistic(ctx context.Context, stat *model.WorkloadStatistic) error {
	return nil
}
func (m *MockDBClient) UpdateWorkloadStatistic(ctx context.Context, stat *model.WorkloadStatistic) error {
	return nil
}
func (m *MockDBClient) DeleteWorkloadStatistic(ctx context.Context, id int32) error { return nil }
func (m *MockDBClient) DeleteWorkloadStatisticsByWorkloadID(ctx context.Context, workloadID string) error {
	return nil
}

func (m *MockDBClient) GetNodeStatisticByID(ctx context.Context, id int32) (*model.NodeStatistic, error) {
	return nil, nil
}
func (m *MockDBClient) GetNodeStatisticByClusterAndNode(ctx context.Context, cluster, nodeName string) (*model.NodeStatistic, error) {
	return nil, nil
}
func (m *MockDBClient) GetNodeStatisticsByCluster(ctx context.Context, cluster string) ([]*model.NodeStatistic, error) {
	return nil, nil
}
func (m *MockDBClient) GetNodeStatisticsByNodeNames(ctx context.Context, cluster string, nodeNames []string) ([]*model.NodeStatistic, error) {
	return nil, nil
}
func (m *MockDBClient) GetNodeGpuUtilizationMap(ctx context.Context, cluster string, nodeNames []string) (map[string]float64, error) {
	return nil, nil
}
func (m *MockDBClient) CreateNodeStatistic(ctx context.Context, stat *model.NodeStatistic) error {
	return nil
}
func (m *MockDBClient) UpdateNodeStatistic(ctx context.Context, stat *model.NodeStatistic) error {
	return nil
}
func (m *MockDBClient) UpsertNodeStatistic(ctx context.Context, stat *model.NodeStatistic) error {
	return nil
}
func (m *MockDBClient) DeleteNodeStatistic(ctx context.Context, id int32) error { return nil }
func (m *MockDBClient) DeleteNodeStatisticByClusterAndNode(ctx context.Context, cluster, nodeName string) error {
	return nil
}
func (m *MockDBClient) DeleteNodeStatisticsByCluster(ctx context.Context, cluster string) error {
	return nil
}

func (m *MockDBClient) UpsertInference(ctx context.Context, inference *dbclient.Inference) error {
	return nil
}
func (m *MockDBClient) SelectInferences(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*dbclient.Inference, error) {
	return nil, nil
}
func (m *MockDBClient) GetInference(ctx context.Context, inferenceId string) (*dbclient.Inference, error) {
	return nil, nil
}
func (m *MockDBClient) CountInferences(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	return 0, nil
}
func (m *MockDBClient) SetInferenceDeleted(ctx context.Context, inferenceId string) error {
	return nil
}

func (m *MockDBClient) InsertPlaygroundSession(ctx context.Context, session *dbclient.PlaygroundSession) error {
	return nil
}
func (m *MockDBClient) UpdatePlaygroundSession(ctx context.Context, session *dbclient.PlaygroundSession) error {
	return nil
}
func (m *MockDBClient) SelectPlaygroundSessions(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*dbclient.PlaygroundSession, error) {
	return nil, nil
}
func (m *MockDBClient) GetPlaygroundSession(ctx context.Context, id int64) (*dbclient.PlaygroundSession, error) {
	return nil, nil
}
func (m *MockDBClient) CountPlaygroundSessions(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	return 0, nil
}
func (m *MockDBClient) SetPlaygroundSessionDeleted(ctx context.Context, id int64) error { return nil }

// Tests

func TestNewTokenRefresher(t *testing.T) {
	ctx := context.Background()
	mockDB := &MockDBClient{}

	refresher := NewTokenRefresher(ctx, mockDB, nil)

	assert.NotNil(t, refresher)
	assert.Equal(t, mockDB, refresher.dbClient)
	assert.NotNil(t, refresher.ctx)
	assert.NotNil(t, refresher.cancelFunc)
	// Default values when config returns 0
	assert.True(t, refresher.interval > 0)
	assert.True(t, refresher.threshold > 0)
}

func TestTokenRefresher_Stop(t *testing.T) {
	ctx := context.Background()
	mockDB := &MockDBClient{}

	refresher := NewTokenRefresher(ctx, mockDB, nil)
	refresher.Stop()

	// Verify context is cancelled
	select {
	case <-refresher.ctx.Done():
		// Expected - context should be cancelled
	default:
		t.Error("Expected context to be cancelled after Stop()")
	}
}

func TestTokenRefresher_StartWithNilDBClient(t *testing.T) {
	ctx := context.Background()

	refresher := &TokenRefresher{
		dbClient:   nil,
		ssoToken:   nil,
		interval:   1 * time.Second,
		threshold:  30 * time.Minute,
		ctx:        ctx,
		cancelFunc: func() {},
	}

	// Should return immediately without panic
	done := make(chan bool)
	go func() {
		refresher.Start()
		done <- true
	}()

	select {
	case <-done:
		// Expected - should return quickly
	case <-time.After(2 * time.Second):
		t.Error("Start() should return immediately when dbClient is nil")
	}
}

func TestTokenRefresher_StartWithNilSSOToken(t *testing.T) {
	ctx := context.Background()
	mockDB := &MockDBClient{}

	refresher := &TokenRefresher{
		dbClient:   mockDB,
		ssoToken:   nil,
		interval:   1 * time.Second,
		threshold:  30 * time.Minute,
		ctx:        ctx,
		cancelFunc: func() {},
	}

	// Should return immediately without panic
	done := make(chan bool)
	go func() {
		refresher.Start()
		done <- true
	}()

	select {
	case <-done:
		// Expected - should return quickly
	case <-time.After(2 * time.Second):
		t.Error("Start() should return immediately when ssoToken is nil")
	}
}

func TestTokenRefresher_RefreshTokensWithNilSSOToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockDB := &MockDBClient{}

	// Note: SelectUserTokens should NOT be called when ssoToken is nil
	// because refreshTokens returns early

	refresher := &TokenRefresher{
		dbClient:   mockDB,
		ssoToken:   nil,
		interval:   20 * time.Minute,
		threshold:  30 * time.Minute,
		ctx:        ctx,
		cancelFunc: cancel,
	}

	// Should return early without calling DB
	refresher.refreshTokens()

	// Verify SelectUserTokens was NOT called
	mockDB.AssertNotCalled(t, "SelectUserTokens", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
