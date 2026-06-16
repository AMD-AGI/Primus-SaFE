/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// dispatchedWorkload builds a dispatched, running workload bound to a node.
func dispatchedWorkload(name, cluster, ws, node string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   name,
		Labels: map[string]string{v1.ClusterIdLabel: cluster, v1.WorkspaceIdLabel: ws},
	}}
	v1.SetAnnotation(w, v1.WorkloadDispatchedAnnotation, v1.TrueStr)
	w.Status.Pods = []v1.WorkloadPod{{AdminNodeName: node}}
	return w
}

// TestWorkspaceValidateNodesRemoved covers running-workload node removal validation.
func TestWorkspaceValidateNodesRemoved(t *testing.T) {
	scheme := newScheme(t)
	wl := dispatchedWorkload("w1", "cluster1", "ws1", "node1")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()
	v := &WorkspaceValidator{Client: k8sClient}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}

	assert.NilError(t, v.validateNodesRemoved(context.Background(), ws, nil))
	assert.Assert(t, v.validateNodesRemoved(context.Background(), ws, []string{"node1"}) != nil)
}

// TestWorkspaceValidateVolumeRemovedConflict covers pvc-in-use removal validation.
func TestWorkspaceValidateVolumeRemovedConflict(t *testing.T) {
	scheme := newScheme(t)
	wl := dispatchedWorkload("w1", "cluster1", "ws1", "node1")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()
	v := &WorkspaceValidator{Client: k8sClient}

	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{
		Cluster: "cluster1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc", Capacity: "100Gi"}},
	}}
	v1.SetLabel(oldWs, v1.ClusterIdLabel, "cluster1")
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetLabel(newWs, v1.ClusterIdLabel, "cluster1")
	assert.Assert(t, v.validateVolumeRemoved(context.Background(), newWs, oldWs) != nil)
}

// TestWorkspaceValidateNodesActionRemove covers node remove action validation.
func TestWorkspaceValidateNodesActionRemove(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{Workspace: pointer.String("ws1")},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	v := &WorkspaceValidator{Client: k8sClient}
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"node1":"remove"}`)
	assert.NilError(t, v.validateNodesAction(context.Background(), newWs, &v1.Workspace{}))
}

// TestWorkspaceMutateNodesActionRemove covers node remove action mutation.
func TestWorkspaceMutateNodesActionRemove(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{
			v1.ClusterIdLabel:    "cluster1",
			v1.NodeFlavorIdLabel: "flavor1",
		}},
		Spec: v1.NodeSpec{Workspace: pointer.String("ws1")},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	m := &WorkspaceMutator{Client: k8sClient}
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", NodeFlavor: "flavor1", Replica: 2}}
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", NodeFlavor: "flavor1", Replica: 2}}
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"node1":"remove"}`)
	assert.NilError(t, m.mutateNodesAction(context.Background(), oldWs, newWs))
	assert.Equal(t, newWs.Spec.Replica, 1)
}

// TestWorkspaceMutateScaleDownSuccess covers successful scale-down node selection.
func TestWorkspaceMutateScaleDownSuccess(t *testing.T) {
	scheme := newScheme(t)
	idleNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "node1",
		Labels: map[string]string{v1.WorkspaceIdLabel: "ws1"},
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(idleNode).Build()
	m := &WorkspaceMutator{Client: k8sClient}
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 2}}
	oldWs.Status.AvailableReplica = 1
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 0}}
	assert.NilError(t, m.mutateScaleDown(context.Background(), oldWs, newWs))
	assert.Assert(t, v1.GetWorkspaceNodesAction(newWs) != "")
}

// TestWorkspaceValidateScaleDownSourceWorkload covers scale-down against a source workload.
func TestWorkspaceValidateScaleDownSourceWorkload(t *testing.T) {
	scheme := newScheme(t)
	src := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "src"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(src).Build()
	v := &WorkspaceValidator{Client: k8sClient}
	oldWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 2}}
	newWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 1}}
	v1.SetLabel(newWs, v1.SourceWorkloadIdLabel, "src")
	assert.Assert(t, v.validateScaleDown(context.Background(), newWs, oldWs) != nil)
}

// TestWorkloadValidateSpecChangedDispatched covers dispatched spec change rejection.
func TestWorkloadValidateSpecChangedDispatched(t *testing.T) {
	v := &WorkloadValidator{}
	oldW := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
		Resources:        []v1.WorkloadResource{wlResource()},
	}}
	v1.SetAnnotation(oldW, v1.WorkloadDispatchedAnnotation, v1.TrueStr)
	newW := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
		Resources:        []v1.WorkloadResource{{Replica: 2, CPU: "2", Memory: "4Gi", EphemeralStorage: "5Gi"}},
	}}
	v1.SetAnnotation(newW, v1.WorkloadDispatchedAnnotation, v1.TrueStr)
	assert.Assert(t, v.validateSpecChanged(newW, oldW) != nil)
}

// TestOpsJobMutateMetaWithNode covers cluster/flavor label derivation from node.
func TestOpsJobMutateMetaWithNode(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "flavor1"}},
	}
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node, cluster).Build()
	m := &OpsJobMutator{Client: k8sClient}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job1"},
		Spec: v1.OpsJobSpec{
			Type:   v1.OpsJobPreflightType,
			Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "node1"}},
		},
	}
	m.mutateMeta(context.Background(), job)
	assert.Equal(t, v1.GetClusterId(job), "cluster1")
	assert.Equal(t, v1.GetNodeFlavorId(job), "flavor1")
}

// TestOpsJobMutateJobSpecWithResource covers resource gpu/replica mutation.
func TestOpsJobMutateJobSpecWithResource(t *testing.T) {
	scheme := newScheme(t)
	flavor := gpuFlavor("flavor1")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(flavor).Build()
	m := &OpsJobMutator{Client: k8sClient}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.NodeFlavorIdLabel: "flavor1"}},
		Spec: v1.OpsJobSpec{
			Resource: &v1.WorkloadResource{CPU: "1", GPU: "8", Memory: "2Gi"},
			Inputs:   []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}},
		},
	}
	m.mutateJobSpec(context.Background(), job)
	assert.Equal(t, job.Spec.Resource.GPUName, common.AmdGpu)
	assert.Equal(t, job.Spec.Resource.Replica, 1)
}

// TestWorkspaceMutatorHandleFull covers the workspace mutator full update path.
func TestWorkspaceMutatorHandleFull(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	m := &WorkspaceMutator{Client: k8sClient, decoder: newDecoder(t)}

	oldWs := validWorkspace("ws1")
	newWs := validWorkspace("ws1")
	newWs.Spec.EnablePreempt = true
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, newWs, oldWs))
	assert.Assert(t, resp.Allowed)
}

// TestNodeFlavorValidatorHandleUpdate covers the node flavor validator update path.
func TestNodeFlavorValidatorHandleUpdate(t *testing.T) {
	v := &NodeFlavorValidator{decoder: newDecoder(t)}
	nf := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: "nf1"},
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory: resource.MustParse("16Gi"),
		},
	}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, nf, nf))
	assert.Assert(t, resp.Allowed)

	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}
