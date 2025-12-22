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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	assert.NotNil(t, svc.clientManager)
}

func TestSetDeploymentFailureCallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	fakeK8s := fake.NewSimpleClientset()

	svc := NewService(mockDB, fakeK8s)

	callbackCalled := false
	callback := func(ctx context.Context, req *dbclient.DeploymentRequest, reason string) {
		callbackCalled = true
	}

	svc.SetDeploymentFailureCallback(callback)
	svc.notifyDeploymentFailure(context.Background(), &dbclient.DeploymentRequest{}, "test failure")

	assert.True(t, callbackCalled)
}

func TestNotifyDeploymentFailureNoCallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	fakeK8s := fake.NewSimpleClientset()

	svc := NewService(mockDB, fakeK8s)

	// Should not panic when callback is nil
	assert.NotPanics(t, func() {
		svc.notifyDeploymentFailure(context.Background(), &dbclient.DeploymentRequest{}, "test")
	})
}

func TestExtractJobNameFromDescription(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	fakeK8s := fake.NewSimpleClientset()

	svc := NewService(mockDB, fakeK8s)

	tests := []struct {
		name        string
		description string
		expected    string
	}{
		{
			name:        "standard format",
			description: "Upgrade deployment | Job: cd-upgrade-123-abc123",
			expected:    "cd-upgrade-123-abc123",
		},
		{
			name:        "job name only",
			description: "Job: cd-upgrade-456-xyz789",
			expected:    "cd-upgrade-456-xyz789",
		},
		{
			name:        "empty description",
			description: "",
			expected:    "",
		},
		{
			name:        "no job prefix",
			description: "Some description without job name",
			expected:    "",
		},
		{
			name:        "job with whitespace",
			description: "Deploy | Job:   cd-remote-789-test  ",
			expected:    "cd-remote-789-test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.extractJobNameFromDescription(tt.description)
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

func TestDeleteJob(t *testing.T) {
	ctx := context.Background()

	t.Run("delete existing job", func(t *testing.T) {
		fakeK8s := fake.NewSimpleClientset(&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "primus-safe",
			},
		})

		svc := &Service{clientSet: fakeK8s}

		err := svc.DeleteJob(ctx, "test-job", "primus-safe")
		assert.NoError(t, err)

		// Verify job is deleted
		_, err = fakeK8s.BatchV1().Jobs("primus-safe").Get(ctx, "test-job", metav1.GetOptions{})
		assert.Error(t, err) // Should not find the job
	})

	t.Run("delete non-existing job returns error", func(t *testing.T) {
		fakeK8s := fake.NewSimpleClientset()
		svc := &Service{clientSet: fakeK8s}

		err := svc.DeleteJob(ctx, "non-existing-job", "primus-safe")
		assert.Error(t, err)
	})
}

func TestFindJobByPrefix(t *testing.T) {
	ctx := context.Background()

	t.Run("find existing job by prefix", func(t *testing.T) {
		fakeK8s := fake.NewSimpleClientset(
			&batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cd-upgrade-123-abc",
					Namespace: "primus-safe",
				},
			},
			&batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cd-remote-123-xyz",
					Namespace: "primus-safe",
				},
			},
			&batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-job",
					Namespace: "primus-safe",
				},
			},
		)

		svc := &Service{clientSet: fakeK8s}

		result := svc.findJobByPrefix(ctx, "cd-remote-123-", "primus-safe")
		assert.Equal(t, "cd-remote-123-xyz", result)
	})

	t.Run("no matching job", func(t *testing.T) {
		fakeK8s := fake.NewSimpleClientset(
			&batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-job",
					Namespace: "primus-safe",
				},
			},
		)

		svc := &Service{clientSet: fakeK8s}

		result := svc.findJobByPrefix(ctx, "cd-remote-", "primus-safe")
		assert.Empty(t, result)
	})
}

func TestBuildRemoteClusterScript(t *testing.T) {
	svc := &Service{}

	t.Run("script with node-agent only", func(t *testing.T) {
		result := &DeploymentResult{
			HasNodeAgent:   true,
			HasCICD:        false,
			NodeAgentImage: "node-agent:v1.0.0",
		}

		script := svc.buildRemoteClusterScript(result)

		assert.Contains(t, script, "HAS_NODE_AGENT=true")
		assert.Contains(t, script, "HAS_CICD=false")
		assert.Contains(t, script, "NODE_AGENT_IMAGE=\"node-agent:v1.0.0\"")
		assert.Contains(t, script, "helm")
	})

	t.Run("script with cicd only", func(t *testing.T) {
		result := &DeploymentResult{
			HasNodeAgent:     false,
			HasCICD:          true,
			CICDRunnerImage:  "cicd-runner:v1.0.0",
			CICDUnifiedImage: "cicd-unified:v1.0.0",
		}

		script := svc.buildRemoteClusterScript(result)

		assert.Contains(t, script, "HAS_NODE_AGENT=false")
		assert.Contains(t, script, "HAS_CICD=true")
		assert.Contains(t, script, "CICD_RUNNER_IMAGE=\"cicd-runner:v1.0.0\"")
		assert.Contains(t, script, "kubectl")
		assert.Contains(t, script, "patch autoscalingrunnersets")
	})

	t.Run("script with both node-agent and cicd", func(t *testing.T) {
		result := &DeploymentResult{
			HasNodeAgent:     true,
			HasCICD:          true,
			NodeAgentImage:   "node-agent:v2.0.0",
			CICDRunnerImage:  "cicd-runner:v2.0.0",
			CICDUnifiedImage: "cicd-unified:v2.0.0",
		}

		script := svc.buildRemoteClusterScript(result)

		assert.Contains(t, script, "HAS_NODE_AGENT=true")
		assert.Contains(t, script, "HAS_CICD=true")
		assert.Contains(t, script, "git clone")
		assert.Contains(t, script, AdminClusterID)
	})
}

func TestVerifyCICDConfigMapUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("verify runner image in ConfigMap", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "github-scale-set-template",
				Namespace: "primus-safe",
			},
			Data: map[string]string{
				"template": `image: harbor.example.com/primussafe/cicd-runner-proxy:v1.0.0`,
			},
		}

		fakeK8s := fake.NewSimpleClientset(cm)
		svc := &Service{clientSet: fakeK8s}

		imageVersions := map[string]string{
			"cicd_runner": "cicd-runner-proxy:v1.0.0",
		}

		err := svc.verifyCICDConfigMapUpdate(ctx, imageVersions)
		assert.NoError(t, err)
	})

	t.Run("image not found in ConfigMap", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "github-scale-set-template",
				Namespace: "primus-safe",
			},
			Data: map[string]string{
				"template": `image: old-image:v0.0.1`,
			},
		}

		fakeK8s := fake.NewSimpleClientset(cm)
		svc := &Service{clientSet: fakeK8s}

		imageVersions := map[string]string{
			"cicd_runner": "cicd-runner-proxy:v2.0.0",
		}

		err := svc.verifyCICDConfigMapUpdate(ctx, imageVersions)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in ConfigMap")
	})

	t.Run("no cicd components to verify", func(t *testing.T) {
		fakeK8s := fake.NewSimpleClientset()
		svc := &Service{clientSet: fakeK8s}

		imageVersions := map[string]string{
			"apiserver": "apiserver:v1.0.0", // Not a CICD component
		}

		err := svc.verifyCICDConfigMapUpdate(ctx, imageVersions)
		assert.NoError(t, err)
	})
}

func TestConstants(t *testing.T) {
	t.Run("verify service constants", func(t *testing.T) {
		assert.Equal(t, "primus-safe", JobNamespace)
		assert.Equal(t, "dtzar/helm-kubectl:latest", JobImage)
		assert.Equal(t, "https://github.com/AMD-AGI/Primus-SaFE.git", PrimusSaFERepoURL)
		assert.Equal(t, "/home/primus-safe-cd", ContainerMountPath)
		assert.Equal(t, "/mnt/primus-safe-cd", HostMountPath)
		assert.Equal(t, "tw-project2", AdminClusterID)
	})
}

func TestDeploymentResult(t *testing.T) {
	t.Run("deployment result initialization", func(t *testing.T) {
		result := &DeploymentResult{
			LocalJobName:     "cd-upgrade-1-abc",
			HasNodeAgent:     true,
			HasCICD:          true,
			NodeAgentImage:   "node-agent:v1",
			CICDRunnerImage:  "runner:v1",
			CICDUnifiedImage: "unified:v1",
		}

		assert.Equal(t, "cd-upgrade-1-abc", result.LocalJobName)
		assert.True(t, result.HasNodeAgent)
		assert.True(t, result.HasCICD)
	})
}

func TestJobParams(t *testing.T) {
	t.Run("job params structure", func(t *testing.T) {
		params := JobParams{
			Name:          "test-job",
			Namespace:     "primus-safe",
			Image:         JobImage,
			ComponentTags: "apiserver.image=v1;",
			NodeAgentTags: "image=v1;",
			EnvFileConfig: "key=value",
		}

		assert.Equal(t, "test-job", params.Name)
		assert.Equal(t, "primus-safe", params.Namespace)
		assert.Contains(t, params.ComponentTags, "apiserver")
	})
}
