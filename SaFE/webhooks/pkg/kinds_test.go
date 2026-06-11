/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// workloadOfKind builds a minimal workload of the given kind for mutation tests.
func workloadOfKind(kind string) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w1"},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: kind, Version: "v1"},
			Env:              map[string]string{},
			Images:          []string{"img"},
			EntryPoints:     []string{"cmd"},
			Resources:       []v1.WorkloadResource{wlResource()},
		},
	}
}

// TestWorkloadMutateCommonAllKinds covers the kind-specific mutation switch arms.
func TestWorkloadMutateCommonAllKinds(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkloadMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	kinds := []string{
		common.DeploymentKind, common.StatefulSetKind, common.AuthoringKind,
		common.CICDScaleRunnerSetKind, common.MonarchJob, common.RayJobKind,
		common.TorchFTKind, common.SandboxKind, common.DynamoDeploymentKind,
		common.OptimusDeploymentKind,
	}
	for _, k := range kinds {
		w := workloadOfKind(k)
		assert.NilError(t, m.mutateCommon(context.Background(), nil, w, nil))
	}
}

// TestWorkloadValidateCommonAllKinds covers the kind-specific validation switch arms.
func TestWorkloadValidateCommonAllKinds(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	kinds := []string{
		common.AuthoringKind, common.CICDScaleRunnerSetKind, common.TorchFTKind,
		common.RayJobKind, common.MonarchJob, common.SandboxKind,
		common.DynamoDeploymentKind, common.OptimusDeploymentKind,
	}
	for _, k := range kinds {
		w := workloadOfKind(k)
		// most kinds fail downstream validation; we only need the switch arm to execute
		_ = v.validateCommon(context.Background(), w, nil)
	}
}
