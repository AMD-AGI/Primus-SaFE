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
)

// TestWorkspaceValidateOnUpdateVolumeRemoved covers the volume-removed update branch.
func TestWorkspaceValidateOnUpdateVolumeRemoved(t *testing.T) {
	scheme := newScheme(t)
	wl := dispatchedWorkload("w1", "cluster1", "ws1", "node1")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()
	v := &WorkspaceValidator{Client: c}

	oldWs := validWorkspace("ws1")
	oldWs.Spec.Volumes = []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc", Capacity: "100Gi"},
	}
	newWs := validWorkspace("ws1")
	assert.Assert(t, v.validateOnUpdate(context.Background(), newWs, oldWs) != nil)
}

// TestWorkspaceMutateCommonGpuProductError covers the gpu product mutation error branch.
func TestWorkspaceMutateCommonGpuProductError(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	m := &WorkspaceMutator{Client: c}
	ws := validWorkspace("ws1")
	ws.Spec.NodeFlavor = "missing"
	v1.SetAnnotation(ws, v1.GpuResourceNameAnnotation, "amd.com/gpu") // skip mutateByNodeFlavor lookup
	assert.Assert(t, m.mutateOnCreation(context.Background(), ws) != nil)
}

// TestOpsJobValidateOnUpdateImmutable covers the ops job update immutable branch.
func TestOpsJobValidateOnUpdateImmutable(t *testing.T) {
	scheme := newScheme(t)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldJob := opsJobWithDisplayName("job1", v1.OpsJobCDType)
	newJob := opsJobWithDisplayName("job1", v1.OpsJobDownloadType) // type changed
	assert.Assert(t, v.validateOnUpdate(context.Background(), newJob, oldJob) != nil)
}
