/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func multiResourceWorkload() *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   "rw",
		Labels: map[string]string{v1.DisplayNameLabel: "disp"},
	}}
	w.Spec.Resources = []v1.WorkloadResource{
		{Replica: 1, CPU: "8", Memory: "16Gi"},
		{Replica: 4, CPU: "8", Memory: "16Gi"},
	}
	w.Spec.EntryPoints = []string{
		stringutil.Base64Encode("entrypoint-0"),
		stringutil.Base64Encode("entrypoint-1"),
	}
	w.Spec.Env = map[string]string{
		common.ReplicaCount:    "2",
		common.MinReplicaCount: "1",
	}
	return w
}

func newGenReconciler(t *testing.T) *DispatcherReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	return &DispatcherReconciler{Client: cl}
}

func TestGenerateLighthouse(t *testing.T) {
	r := newGenReconciler(t)
	w := multiResourceWorkload()
	lh := r.generateLighthouse(context.Background(), w)
	assert.Assert(t, lh != nil)
	assert.Equal(t, lh.Name, "rw-0")
	assert.Equal(t, string(lh.Spec.Kind), common.DeploymentKind)
	assert.Assert(t, lh.Spec.Service != nil)
}

func TestGenerateTorchFTWorker(t *testing.T) {
	r := newGenReconciler(t)
	w := multiResourceWorkload()
	worker := r.generateTorchFTWorker(context.Background(), w, 0, 2, "lighthouse:29400")
	assert.Assert(t, worker != nil)
	assert.Equal(t, worker.Name, "rw-1")
	assert.Equal(t, string(worker.Spec.Kind), common.PytorchJobKind)
	assert.Equal(t, worker.Spec.Env[common.TorchFTLightHouse], "lighthouse:29400")
}

func TestGenerateMonarchClient(t *testing.T) {
	r := newGenReconciler(t)
	w := multiResourceWorkload()
	client := r.generateMonarchClient(context.Background(), w, 2)
	assert.Assert(t, client != nil)
	assert.Equal(t, client.Name, "rw")
	assert.Equal(t, string(client.Spec.Kind), common.MonarchClient)
}

func TestGenerateMonarchMesh(t *testing.T) {
	r := newGenReconciler(t)
	w := multiResourceWorkload()
	mesh := r.generateMonarchMesh(context.Background(), w, 2, 0)
	assert.Assert(t, mesh != nil)
	assert.Equal(t, string(mesh.Spec.Kind), common.MonarchMesh)
}
