/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cdhandlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

func TestNewService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	fakeK8s := fake.NewSimpleClientset()

	svc := NewService(mockDB, fakeK8s)

	assert.NotNil(t, svc)
	assert.NotNil(t, svc.clientSet)
}

func TestExtractBranchFromEnvFileConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard format",
			input:    "deploy_branch=main",
			expected: "main",
		},
		{
			name:     "with quotes",
			input:    "deploy_branch=\"feature/test\"",
			expected: "feature/test",
		},
		{
			name:     "multiline config",
			input:    "some_key=value\ndeploy_branch=develop\nother=x",
			expected: "develop",
		},
		{
			name:     "not found",
			input:    "other_key=value",
			expected: "",
		},
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBranchFromEnvFileConfig(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractInstallNodeAgentFromEnvFileConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "install enabled",
			input:    "install_node_agent=y",
			expected: true,
		},
		{
			name:     "install disabled",
			input:    "install_node_agent=n",
			expected: false,
		},
		{
			name:     "not found",
			input:    "other_key=value",
			expected: false,
		},
		{
			name:     "with quotes",
			input:    "install_node_agent=\"y\"",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractInstallNodeAgentFromEnvFileConfig(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCvtDBRequestToItem(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	fakeK8s := fake.NewSimpleClientset()

	svc := NewService(mockDB, fakeK8s)

	now := time.Now().UTC()

	t.Run("convert full request", func(t *testing.T) {
		req := &dbclient.DeploymentRequest{
			Id:              123,
			DeployName:      "test-user",
			Status:          StatusDeployed,
			ApproverName:    dbutils.NullString("admin"),
			ApprovalResult:  dbutils.NullString(StatusApproved),
			Description:     dbutils.NullString("Test deployment"),
			RejectionReason: sql.NullString{Valid: false},
			FailureReason:   sql.NullString{Valid: false},
			RollbackFromId:  sql.NullInt64{Int64: 100, Valid: true},
			CreatedAt:       dbutils.NullTime(now),
			UpdatedAt:       dbutils.NullTime(now),
			ApprovedAt:      dbutils.NullTime(now),
		}

		item := svc.cvtDBRequestToItem(req)

		assert.Equal(t, int64(123), item.Id)
		assert.Equal(t, "test-user", item.DeployName)
		assert.Equal(t, StatusDeployed, item.Status)
		assert.Equal(t, "admin", item.ApproverName)
		assert.Equal(t, StatusApproved, item.ApprovalResult)
		assert.Equal(t, "Test deployment", item.Description)
		assert.Equal(t, int64(100), item.RollbackFromId)
	})

	t.Run("convert request with null fields", func(t *testing.T) {
		req := &dbclient.DeploymentRequest{
			Id:         456,
			DeployName: "user2",
			Status:     StatusPendingApproval,
		}

		item := svc.cvtDBRequestToItem(req)

		assert.Equal(t, int64(456), item.Id)
		assert.Empty(t, item.ApproverName)
		assert.Empty(t, item.RejectionReason)
	})
}

func TestUpdateRequestStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	t.Run("update status successfully", func(t *testing.T) {
		mockDB := mock_client.NewMockInterface(ctrl)
		fakeK8s := fake.NewSimpleClientset()
		svc := NewService(mockDB, fakeK8s)

		req := &dbclient.DeploymentRequest{
			Id:         1,
			DeployName: "test",
			Status:     StatusDeploying,
		}

		mockDB.EXPECT().GetDeploymentRequest(ctx, int64(1)).Return(req, nil)
		mockDB.EXPECT().UpdateDeploymentRequest(ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, r *dbclient.DeploymentRequest) error {
				assert.Equal(t, StatusDeployed, r.Status)
				return nil
			})

		err := svc.UpdateRequestStatus(ctx, 1, StatusDeployed, "")
		assert.NoError(t, err)
	})

	t.Run("update status with failure reason", func(t *testing.T) {
		mockDB := mock_client.NewMockInterface(ctrl)
		fakeK8s := fake.NewSimpleClientset()
		svc := NewService(mockDB, fakeK8s)

		req := &dbclient.DeploymentRequest{
			Id:         2,
			DeployName: "test",
			Status:     StatusDeploying,
		}

		mockDB.EXPECT().GetDeploymentRequest(ctx, int64(2)).Return(req, nil)
		mockDB.EXPECT().UpdateDeploymentRequest(ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, r *dbclient.DeploymentRequest) error {
				assert.Equal(t, StatusFailed, r.Status)
				assert.Equal(t, "Pod crash", r.FailureReason.String)
				return nil
			})

		err := svc.UpdateRequestStatus(ctx, 2, StatusFailed, "Pod crash")
		assert.NoError(t, err)
	})
}

func TestCreateSnapshot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	t.Run("create snapshot with new config", func(t *testing.T) {
		mockDB := mock_client.NewMockInterface(ctrl)
		fakeK8s := fake.NewSimpleClientset()
		svc := NewService(mockDB, fakeK8s)

		newConfig := DeploymentConfig{
			ImageVersions: map[string]string{
				"apiserver": "v2.0.0",
			},
			EnvFileConfig: "new_env=value",
		}
		configJSON, _ := json.Marshal(newConfig)

		// No previous snapshot
		mockDB.EXPECT().ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0).
			Return([]*dbclient.EnvironmentSnapshot{}, nil)

		mockDB.EXPECT().CreateEnvironmentSnapshot(ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, s *dbclient.EnvironmentSnapshot) (int64, error) {
				assert.Equal(t, int64(1), s.DeploymentRequestId)

				var savedConfig DeploymentConfig
				err := json.Unmarshal([]byte(s.EnvConfig), &savedConfig)
				require.NoError(t, err)

				assert.Equal(t, "v2.0.0", savedConfig.ImageVersions["apiserver"])
				assert.Equal(t, "new_env=value", savedConfig.EnvFileConfig)
				return 1, nil
			})

		err := svc.CreateSnapshot(ctx, 1, string(configJSON))
		assert.NoError(t, err)
	})

	t.Run("create snapshot merging with previous", func(t *testing.T) {
		mockDB := mock_client.NewMockInterface(ctrl)
		fakeK8s := fake.NewSimpleClientset()
		svc := NewService(mockDB, fakeK8s)

		previousConfig := DeploymentConfig{
			ImageVersions: map[string]string{
				"apiserver":        "v1.0.0",
				"resource_manager": "v1.0.0",
			},
			EnvFileConfig: "old_env=value",
		}
		previousConfigJSON, _ := json.Marshal(previousConfig)

		newConfig := DeploymentConfig{
			ImageVersions: map[string]string{
				"apiserver": "v2.0.0", // Update apiserver only
			},
			// No env file config - should use previous
		}
		newConfigJSON, _ := json.Marshal(newConfig)

		mockDB.EXPECT().ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0).
			Return([]*dbclient.EnvironmentSnapshot{
				{Id: 1, EnvConfig: string(previousConfigJSON)},
			}, nil)

		mockDB.EXPECT().CreateEnvironmentSnapshot(ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, s *dbclient.EnvironmentSnapshot) (int64, error) {
				var mergedConfig DeploymentConfig
				err := json.Unmarshal([]byte(s.EnvConfig), &mergedConfig)
				require.NoError(t, err)

				// apiserver should be updated
				assert.Equal(t, "v2.0.0", mergedConfig.ImageVersions["apiserver"])
				// resource_manager should be preserved
				assert.Equal(t, "v1.0.0", mergedConfig.ImageVersions["resource_manager"])
				// env file should be preserved from previous
				assert.Equal(t, "old_env=value", mergedConfig.EnvFileConfig)
				return 2, nil
			})

		err := svc.CreateSnapshot(ctx, 2, string(newConfigJSON))
		assert.NoError(t, err)
	})
}

func TestMergeWithLatestSnapshot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	t.Run("merge with existing snapshot", func(t *testing.T) {
		mockDB := mock_client.NewMockInterface(ctrl)
		fakeK8s := fake.NewSimpleClientset()
		svc := NewService(mockDB, fakeK8s)

		snapshotConfig := DeploymentConfig{
			ImageVersions: map[string]string{
				"apiserver":        "v1.0.0",
				"resource_manager": "v1.0.0",
				"job_manager":      "v1.0.0",
			},
			EnvFileConfig: "snapshot_env=1",
		}
		snapshotJSON, _ := json.Marshal(snapshotConfig)

		mockDB.EXPECT().ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0).
			Return([]*dbclient.EnvironmentSnapshot{
				{Id: 1, EnvConfig: string(snapshotJSON)},
			}, nil)

		currentConfig := DeploymentConfig{
			ImageVersions: map[string]string{
				"apiserver": "v2.0.0", // Only updating apiserver
			},
			EnvFileConfig: "", // No env file update
		}

		merged, err := svc.mergeWithLatestSnapshot(ctx, currentConfig)
		require.NoError(t, err)

		// apiserver should be updated
		assert.Equal(t, "v2.0.0", merged.ImageVersions["apiserver"])
		// Others should be preserved
		assert.Equal(t, "v1.0.0", merged.ImageVersions["resource_manager"])
		assert.Equal(t, "v1.0.0", merged.ImageVersions["job_manager"])
		// Env file should come from snapshot
		assert.Equal(t, "snapshot_env=1", merged.EnvFileConfig)
	})

	t.Run("no snapshot available", func(t *testing.T) {
		mockDB := mock_client.NewMockInterface(ctrl)
		fakeK8s := fake.NewSimpleClientset()
		svc := NewService(mockDB, fakeK8s)

		mockDB.EXPECT().ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0).
			Return([]*dbclient.EnvironmentSnapshot{}, nil)

		currentConfig := DeploymentConfig{
			ImageVersions: map[string]string{
				"apiserver": "v1.0.0",
			},
			EnvFileConfig: "new_env=1",
		}

		merged, err := svc.mergeWithLatestSnapshot(ctx, currentConfig)
		require.NoError(t, err)

		// Should return current config as-is
		assert.Equal(t, currentConfig.ImageVersions, merged.ImageVersions)
		assert.Equal(t, currentConfig.EnvFileConfig, merged.EnvFileConfig)
	})
}

func TestRollback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	t.Run("rollback from deployed request with snapshot", func(t *testing.T) {
		mockDB := mock_client.NewMockInterface(ctrl)
		fakeK8s := fake.NewSimpleClientset()
		svc := NewService(mockDB, fakeK8s)

		targetReq := &dbclient.DeploymentRequest{
			Id:        10,
			Status:    StatusDeployed,
			EnvConfig: `{"image_versions":{"apiserver":"v1.0.0"}}`,
		}

		snapshotConfig := `{"image_versions":{"apiserver":"v1.0.0","resource_manager":"v1.0.0"},"env_file_config":"full_env"}`
		snapshot := &dbclient.EnvironmentSnapshot{
			Id:                  1,
			DeploymentRequestId: 10,
			EnvConfig:           snapshotConfig,
		}

		mockDB.EXPECT().GetDeploymentRequest(ctx, int64(10)).Return(targetReq, nil)
		mockDB.EXPECT().GetEnvironmentSnapshotByRequestId(ctx, int64(10)).Return(snapshot, nil)
		mockDB.EXPECT().CreateDeploymentRequest(ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, r *dbclient.DeploymentRequest) (int64, error) {
				assert.Equal(t, "rollback-user", r.DeployName)
				assert.Equal(t, StatusPendingApproval, r.Status)
				assert.Equal(t, snapshotConfig, r.EnvConfig)
				assert.Equal(t, int64(10), r.RollbackFromId.Int64)
				return 11, nil
			})

		newId, err := svc.Rollback(ctx, 10, "rollback-user")
		require.NoError(t, err)
		assert.Equal(t, int64(11), newId)
	})

	t.Run("cannot rollback non-deployed request", func(t *testing.T) {
		mockDB := mock_client.NewMockInterface(ctrl)
		fakeK8s := fake.NewSimpleClientset()
		svc := NewService(mockDB, fakeK8s)

		targetReq := &dbclient.DeploymentRequest{
			Id:     5,
			Status: StatusPendingApproval, // Not deployed
		}

		mockDB.EXPECT().GetDeploymentRequest(ctx, int64(5)).Return(targetReq, nil)

		_, err := svc.Rollback(ctx, 5, "user")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot rollback")
	})
}

func TestConstants(t *testing.T) {
	t.Run("verify service constants", func(t *testing.T) {
		assert.Equal(t, "/mnt/primus-safe-cd", HostMountPath)
	})
}
