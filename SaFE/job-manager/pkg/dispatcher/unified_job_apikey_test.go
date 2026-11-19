/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// unifiedJobForTest is a UnifiedJob of user-1 in workspace ws on cluster c1 started by scale set "scale-set".
func unifiedJobForTest() *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "uj", Labels: map[string]string{v1.UserIdLabel: "user-1", v1.ClusterIdLabel: "c1"}}}
	w.Spec.GroupVersionKind.Kind = common.UnifiedJobKind
	w.Spec.Workspace = "ws"
	w.Spec.Env = map[string]string{common.ScaleRunnerSetID: "scale-set"}
	return w
}

// scaleSetForUnifiedJob is the scale set of unifiedJobForTest carrying enc as its encrypted API key.
func scaleSetForUnifiedJob(enc string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "scale-set", Labels: map[string]string{v1.UserIdLabel: "user-1", v1.ClusterIdLabel: "c1"}}}
	w.Spec.GroupVersionKind.Kind = common.CICDScaleRunnerSetKind
	w.Spec.Workspace = "ws"
	if enc != "" {
		v1.SetAnnotation(w, v1.UserEncryptedAPIKeyAnnotation, enc)
	}
	return w
}

func TestUnifiedJobTakesTheScaleSetAPIKey(t *testing.T) {
	stubPlatformKey(t, "db-key")
	parent := scaleSetForUnifiedJob("scale-set-key")

	derived := cicdWorkloadWithParentAPIKey(unifiedJobForTest(), parent)
	envs, err := buildEnvironment(derived, nil, -1)
	assert.NilError(t, err)
	assert.Equal(t, true, findEnv(envs, jobutils.UserApiKeyEnv, "scale-set-key"))
	assert.Equal(t, false, findEnv(envs, jobutils.UserApiKeyEnv, "db-key"))

	foreign := unifiedJobForTest()
	foreign.Labels[v1.UserIdLabel] = "user-2"
	assert.Assert(t, cicdWorkloadWithParentAPIKey(foreign, parent) == foreign, "a job of another user gets no key")

	envs, err = buildEnvironment(unifiedJobForTest(), nil, -1)
	assert.NilError(t, err)
	assert.Equal(t, true, findEnv(envs, jobutils.UserApiKeyEnv, "db-key"), "a job without a scale set key keeps the database key")
}

func TestUnifiedJobScaleSetResolution(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	build := func(objs ...ctrlclient.Object) ctrlclient.Client {
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	}
	ctx := context.Background()
	job := unifiedJobForTest()
	resolve := func(c ctrlclient.Client, w *v1.Workload) *v1.Workload {
		got, err := unifiedJobScaleSet(ctx, c, w)
		assert.NilError(t, err)
		return got
	}

	got := resolve(build(scaleSetForUnifiedJob("k")), job)
	assert.Assert(t, got != nil && got.Name == "scale-set")

	assert.Assert(t, resolve(build(), job) == nil, "a missing scale set")

	otherWs := scaleSetForUnifiedJob("k")
	otherWs.Spec.Workspace = "ws2"
	assert.Assert(t, resolve(build(otherWs), job) == nil, "a scale set of another workspace")

	otherCluster := scaleSetForUnifiedJob("k")
	otherCluster.Labels[v1.ClusterIdLabel] = "c2"
	assert.Assert(t, resolve(build(otherCluster), job) == nil, "a scale set of another cluster")

	notScaleSet := scaleSetForUnifiedJob("k")
	notScaleSet.Spec.GroupVersionKind.Kind = common.PytorchJobKind
	assert.Assert(t, resolve(build(notScaleSet), job) == nil, "not a scale set")

	noEnv := unifiedJobForTest()
	noEnv.Spec.Env = nil
	assert.Assert(t, resolve(build(scaleSetForUnifiedJob("k")), noEnv) == nil, "a job not started by a scale set")
}

func TestUnifiedJobScaleSetReadErrorPropagates(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	failing := fake.NewClientBuilder().WithScheme(scheme).WithObjects(scaleSetForUnifiedJob("k")).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, ctrlclient.WithWatch, ctrlclient.ObjectKey, ctrlclient.Object, ...ctrlclient.GetOption) error {
				return errors.New("apiserver unavailable")
			},
		}).Build()

	got, err := unifiedJobScaleSet(context.Background(), failing, unifiedJobForTest())
	assert.ErrorContains(t, err, "apiserver unavailable")
	assert.Assert(t, got == nil)
}
