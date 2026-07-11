/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"fmt"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/apikey"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func TestPlatformKeyForUser(t *testing.T) {
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-workload",
			Labels: map[string]string{
				v1.UserIdLabel: "user-1",
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: "alice",
			},
		},
	}

	t.Run("db disabled", func(t *testing.T) {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(commonconfig.IsDBEnable, func() bool { return false })

		assert.Equal(t, "", platformKeyForUser(workload))
	})

	t.Run("empty user id", func(t *testing.T) {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(commonconfig.IsDBEnable, func() bool { return true })

		assert.Equal(t, "", platformKeyForUser(&v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}))
	})

	t.Run("db client unavailable", func(t *testing.T) {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(commonconfig.IsDBEnable, func() bool { return true })
		patches.ApplyFunc(dbclient.NewClient, func() *dbclient.Client { return nil })

		assert.Equal(t, "", platformKeyForUser(workload))
	})

	t.Run("lookup error", func(t *testing.T) {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(commonconfig.IsDBEnable, func() bool { return true })
		patches.ApplyFunc(dbclient.NewClient, func() *dbclient.Client { return &dbclient.Client{} })
		patches.ApplyFunc(apikey.GetOrCreatePlatformKey, func(context.Context, dbclient.Interface, string, string) (string, error) {
			return "", fmt.Errorf("lookup failed")
		})

		assert.Equal(t, "", platformKeyForUser(workload))
	})

	t.Run("success", func(t *testing.T) {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(commonconfig.IsDBEnable, func() bool { return true })
		patches.ApplyFunc(dbclient.NewClient, func() *dbclient.Client { return &dbclient.Client{} })
		patches.ApplyFunc(apikey.GetOrCreatePlatformKey, func(_ context.Context, _ dbclient.Interface, userId, userName string) (string, error) {
			assert.Equal(t, "user-1", userId)
			assert.Equal(t, "alice", userName)
			return "platform-token-for-user", nil
		})

		assert.Equal(t, "platform-token-for-user", platformKeyForUser(workload))
	})
}

// stubPlatformKey overrides the platformKeyForUserFn seam for a test and
// restores it on cleanup. Assigning the package var is deterministic, unlike
// gomonkey patching of the cross-package apikey/db chain, which proved flaky
// when many tests patch it in a single package run.
func stubPlatformKey(t *testing.T, key string) {
	t.Helper()
	orig := platformKeyForUserFn
	platformKeyForUserFn = func(*v1.Workload) string { return key }
	t.Cleanup(func() { platformKeyForUserFn = orig })
}

func TestBuildEnvironment_InjectsUserIdAndApiKey(t *testing.T) {
	stubPlatformKey(t, "injected-user-api-key")

	// UnifiedJob workload satisfies the buildEnvironment gate
	// (IsCICD || UnifiedJobKind); it must get USER_ID + USER_APIKEY.
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "utd-multi-node-test",
			Labels: map[string]string{v1.UserIdLabel: "user-1"},
		},
	}
	workload.Spec.GroupVersionKind.Kind = common.UnifiedJobKind
	workload.Spec.Workspace = "ws"

	envs := buildEnvironment(workload, nil, -1)
	assert.Equal(t, true, findEnv(envs, jobutils.UserIdEnv, "user-1"))
	assert.Equal(t, true, findEnv(envs, jobutils.UserApiKeyEnv, "injected-user-api-key"))
}

func TestUpdateCICDScaleSetEnvs_InjectsUserApiKey(t *testing.T) {
	stubPlatformKey(t, "injected-user-api-key")

	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Labels[v1.UserIdLabel] = "user-cicd"
	workload.Spec.Env[common.GithubConfigUrl] = "https://github.com/test/repo"
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, "test-github-secret")
	v1.SetAnnotation(workload, v1.AdminControlPlaneAnnotation, "10.0.0.1")
	v1.SetAnnotation(workload, v1.MainContainerAnnotation, "runner")
	workload.Spec.Workspace = workspace.Name

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name": "runner",
							"env":  []interface{}{},
						},
					},
				},
			},
		},
	}}

	resourceSpec := jobutils.TestCICDScaleSetResourceTemplate.Spec.ResourceSpecs[0]
	err := updateCICDScaleSetEnvs(obj, workload, workspace, resourceSpec)
	assert.NilError(t, err)

	envs := getEnvs(t, obj, workload, &resourceSpec)
	assert.Equal(t, true, findEnv(envs, jobutils.UserApiKeyEnv, "injected-user-api-key"))
}
