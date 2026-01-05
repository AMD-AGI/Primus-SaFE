/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestCreateSnapshot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	t.Run("create snapshot with new config merges with previous", func(t *testing.T) {
		mockDB := mock_client.NewMockInterface(ctrl)

		// Previous snapshot with existing components
		previousConfig := deploymentConfig{
			ImageVersions: map[string]string{
				"apiserver":        "v1.0.0",
				"resource_manager": "v1.0.0",
				"job_manager":      "v1.0.0",
			},
			EnvFileConfig: "old_env=value",
		}
		previousJSON, _ := json.Marshal(previousConfig)

		mockDB.EXPECT().ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0).
			Return([]*dbclient.EnvironmentSnapshot{
				{Id: 1, EnvConfig: string(previousJSON)},
			}, nil)

		mockDB.EXPECT().CreateEnvironmentSnapshot(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, s *dbclient.EnvironmentSnapshot) (int64, error) {
				var savedConfig deploymentConfig
				err := json.Unmarshal([]byte(s.EnvConfig), &savedConfig)
				require.NoError(t, err)

				// apiserver should be updated to v2.0.0
				assert.Equal(t, "v2.0.0", savedConfig.ImageVersions["apiserver"])
				// resource_manager should be preserved from previous
				assert.Equal(t, "v1.0.0", savedConfig.ImageVersions["resource_manager"])
				// job_manager should be preserved from previous
				assert.Equal(t, "v1.0.0", savedConfig.ImageVersions["job_manager"])
				// env file should be preserved from previous (new config has empty)
				assert.Equal(t, "old_env=value", savedConfig.EnvFileConfig)
				return 2, nil
			})

		r := &CDOpsJobReconciler{dbClient: mockDB}

		// New config only updates apiserver
		newConfig := deploymentConfig{
			ImageVersions: map[string]string{
				"apiserver": "v2.0.0",
			},
			EnvFileConfig: "", // Empty, should preserve previous
		}
		newJSON, _ := json.Marshal(newConfig)

		err := r.createSnapshot(ctx, 1, string(newJSON))
		assert.NoError(t, err)
	})

	t.Run("create snapshot with new env file config", func(t *testing.T) {
		mockDB := mock_client.NewMockInterface(ctrl)

		previousConfig := deploymentConfig{
			ImageVersions: map[string]string{
				"apiserver": "v1.0.0",
			},
			EnvFileConfig: "old_env=value",
		}
		previousJSON, _ := json.Marshal(previousConfig)

		mockDB.EXPECT().ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0).
			Return([]*dbclient.EnvironmentSnapshot{
				{Id: 1, EnvConfig: string(previousJSON)},
			}, nil)

		mockDB.EXPECT().CreateEnvironmentSnapshot(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, s *dbclient.EnvironmentSnapshot) (int64, error) {
				var savedConfig deploymentConfig
				err := json.Unmarshal([]byte(s.EnvConfig), &savedConfig)
				require.NoError(t, err)

				// env file should be updated to new value
				assert.Equal(t, "new_env=value", savedConfig.EnvFileConfig)
				return 2, nil
			})

		r := &CDOpsJobReconciler{dbClient: mockDB}

		newConfig := deploymentConfig{
			ImageVersions: map[string]string{},
			EnvFileConfig: "new_env=value",
		}
		newJSON, _ := json.Marshal(newConfig)

		err := r.createSnapshot(ctx, 1, string(newJSON))
		assert.NoError(t, err)
	})

	t.Run("create snapshot without previous snapshot", func(t *testing.T) {
		mockDB := mock_client.NewMockInterface(ctrl)

		// No previous snapshots
		mockDB.EXPECT().ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0).
			Return([]*dbclient.EnvironmentSnapshot{}, nil)

		mockDB.EXPECT().CreateEnvironmentSnapshot(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, s *dbclient.EnvironmentSnapshot) (int64, error) {
				var savedConfig deploymentConfig
				err := json.Unmarshal([]byte(s.EnvConfig), &savedConfig)
				require.NoError(t, err)

				// Only new config should be present
				assert.Equal(t, "v1.0.0", savedConfig.ImageVersions["apiserver"])
				assert.Equal(t, "new_env=value", savedConfig.EnvFileConfig)
				return 1, nil
			})

		r := &CDOpsJobReconciler{dbClient: mockDB}

		newConfig := deploymentConfig{
			ImageVersions: map[string]string{
				"apiserver": "v1.0.0",
			},
			EnvFileConfig: "new_env=value",
		}
		newJSON, _ := json.Marshal(newConfig)

		err := r.createSnapshot(ctx, 1, string(newJSON))
		assert.NoError(t, err)
	})
}

func TestGetJobFailureReason(t *testing.T) {
	// Test the failure reason extraction from OpsJob
	// This is a simple function test
}
