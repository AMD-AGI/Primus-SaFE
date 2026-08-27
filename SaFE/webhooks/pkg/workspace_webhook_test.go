/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"strings"
	"testing"

	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// validWorkspace builds a workspace passing required-params validation.
func validWorkspace(name string) *v1.Workspace {
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.WorkspaceSpec{
			Cluster:     "cluster1",
			QueuePolicy: v1.QueueFifoPolicy,
		},
	}
	v1.SetLabel(ws, v1.ClusterIdLabel, "cluster1")
	v1.SetLabel(ws, v1.DisplayNameLabel, "my-ws")
	return ws
}

// TestWorkspaceMutateQueuePolicy verifies default queue policy assignment.
func TestWorkspaceMutateQueuePolicy(t *testing.T) {
	m := &WorkspaceMutator{}
	ws := &v1.Workspace{}
	m.mutateQueuePolicy(ws)
	assert.Equal(t, ws.Spec.QueuePolicy, v1.QueueFifoPolicy)
}

// TestWorkspaceMutateVolumes verifies volume id assignment and path normalization.
func TestWorkspaceMutateVolumes(t *testing.T) {
	m := &WorkspaceMutator{}
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{HostPath: "/data/", SubPath: "/sub/"},
	}}}
	m.mutateVolumes(ws)
	assert.Equal(t, ws.Spec.Volumes[0].Id, 1)
	assert.Equal(t, ws.Spec.Volumes[0].MountPath, "/data")
	assert.Equal(t, ws.Spec.Volumes[0].SubPath, "sub")
	assert.Equal(t, ws.Spec.Volumes[0].AccessMode, corev1.ReadWriteMany)
}

// TestIsMaxRuntimeEqual verifies max runtime comparison.
func TestIsMaxRuntimeEqual(t *testing.T) {
	a := map[v1.WorkspaceScope]int{v1.TrainScope: 1}
	b := map[v1.WorkspaceScope]int{v1.TrainScope: 1}
	assert.Assert(t, isMaxRuntimeEqual(a, b))
	assert.Assert(t, !isMaxRuntimeEqual(a, map[v1.WorkspaceScope]int{v1.TrainScope: 2}))
	assert.Assert(t, !isMaxRuntimeEqual(a, map[v1.WorkspaceScope]int{}))
}

// TestWorkspaceMutateByNodeFlavor verifies replica reset and gpu annotation.
func TestWorkspaceMutateByNodeFlavor(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 5}}
	assert.NilError(t, m.mutateByNodeFlavor(context.Background(), ws))
	assert.Equal(t, ws.Spec.Replica, 0)
}

// TestWorkspaceMutateMeta verifies labels and finalizer on workspace.
func TestWorkspaceMutateMeta(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	m := &WorkspaceMutator{Client: k8sClient}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "WS1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	assert.NilError(t, m.mutateMeta(context.Background(), ws))
	assert.Equal(t, v1.GetClusterId(ws), "cluster1")
	assert.Equal(t, v1.GetWorkspaceId(ws), "ws1")
}

// TestWorkspaceMutateGpuProduct verifies gpu product annotation from node flavor.
func TestWorkspaceMutateGpuProduct(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	ws := &v1.Workspace{}
	assert.NilError(t, m.mutateGpuProduct(context.Background(), ws))
}

// TestWorkspaceMutateDefaultWorkspaceUsers verifies default-workspace user assignment.
func TestWorkspaceMutateDefaultWorkspaceUsers(t *testing.T) {
	scheme := newScheme(t)
	user := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user).Build()
	m := &WorkspaceMutator{Client: k8sClient}

	notDefault := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	assert.NilError(t, m.mutateDefaultWorkspaceUsers(context.Background(), nil, notDefault))

	isDefault := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{IsDefault: true}}
	assert.NilError(t, m.mutateDefaultWorkspaceUsers(context.Background(), nil, isDefault))
	updated := &v1.User{}
	assert.NilError(t, k8sClient.Get(context.Background(), client.ObjectKey{Name: "u1"}, updated))
	assert.Assert(t, commonuser.HasWorkspaceRight(updated, "ws1"))
}

// TestWorkspaceMutateScaleDown verifies no-op when not scaling down.
func TestWorkspaceMutateScaleDown(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 1}}
	newWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 2}}
	assert.NilError(t, m.mutateScaleDown(context.Background(), oldWs, newWs))
}

// TestWorkspaceMutateOnCreation verifies the full create mutation path.
func TestWorkspaceMutateOnCreation(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	m := &WorkspaceMutator{Client: k8sClient}
	assert.NilError(t, m.mutateOnCreation(context.Background(), validWorkspace("ws1")))
}

// TestWorkspaceMutatorHandle verifies the workspace mutator admission handler.
func TestWorkspaceMutatorHandle(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	m := &WorkspaceMutator{Client: k8sClient, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, validWorkspace("ws1"), nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Update, validWorkspace("ws1"), validWorkspace("ws1")))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Delete, validWorkspace("ws1"), nil))
	assert.Assert(t, resp.Allowed)
}

// TestWorkspaceValidateRequiredParams verifies required-params validation.
func TestWorkspaceValidateRequiredParams(t *testing.T) {
	v := &WorkspaceValidator{}
	assert.NilError(t, v.validateRequiredParams(validWorkspace("ws1")))
	assert.Assert(t, v.validateRequiredParams(&v1.Workspace{}) != nil)

	reserved := validWorkspace(corev1.NamespaceDefault)
	assert.Assert(t, v.validateRequiredParams(reserved) != nil)
}

// TestWorkspaceValidateVolumes verifies volume validation rules.
func TestWorkspaceValidateVolumes(t *testing.T) {
	v := &WorkspaceValidator{}
	hostpath := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Type: v1.HOSTPATH, MountPath: "/data", HostPath: "/data"},
	}}}
	assert.NilError(t, v.validateVolumes(hostpath, nil))

	noMount := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{{Type: v1.HOSTPATH}}}}
	assert.Assert(t, v.validateVolumes(noMount, nil) != nil)

	pfs := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc", Capacity: "100Gi", AccessMode: corev1.ReadWriteMany},
	}}}
	assert.NilError(t, v.validateVolumes(pfs, nil))
}

// TestWorkspaceValidateImmutableFields verifies cluster immutability.
func TestWorkspaceValidateImmutableFields(t *testing.T) {
	v := &WorkspaceValidator{}
	oldWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	same := &v1.Workspace{Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	assert.NilError(t, v.validateImmutableFields(same, oldWs))
	changed := &v1.Workspace{Spec: v1.WorkspaceSpec{Cluster: "cluster2"}}
	assert.Assert(t, v.validateImmutableFields(changed, oldWs) != nil)
}

// TestWorkspaceValidateRelatedResource verifies related resource existence checks.
func TestWorkspaceValidateRelatedResource(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	noFlavor := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 0}}
	assert.NilError(t, v.validateRelatedResource(context.Background(), noFlavor))

	missing := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 1, NodeFlavor: "x", Cluster: "c"}}
	assert.Assert(t, v.validateRelatedResource(context.Background(), missing) != nil)
}

// TestParseNodesAction verifies node action annotation parsing.
func TestParseNodesAction(t *testing.T) {
	empty := &v1.Workspace{}
	actions, err := commonnodes.ParseAction(empty)
	assert.NilError(t, err)
	assert.Assert(t, actions == nil)

	ws := &v1.Workspace{}
	v1.SetAnnotation(ws, v1.WorkspaceNodesAction, `{"node1":"add"}`)
	actions, err = commonnodes.ParseAction(ws)
	assert.NilError(t, err)
	assert.Equal(t, actions["node1"], "add")

	bad := &v1.Workspace{}
	v1.SetAnnotation(bad, v1.WorkspaceNodesAction, `{invalid`)
	_, err = commonnodes.ParseAction(bad)
	assert.Assert(t, err != nil)
}

// TestWorkspaceMutateNodesAction verifies node add action adjusts workspace replica.
func TestWorkspaceMutateNodesAction(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			Labels: map[string]string{
				v1.ClusterIdLabel:    "cluster1",
				v1.NodeFlavorIdLabel: "flavor1",
			},
		},
		// Onboarded: the mutator turns an add of an unmanaged node away before it can move
		// Spec.Replica, so a node this test expects to be addable has to be one.
		Status: v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	m := &WorkspaceMutator{Client: k8sClient}

	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"node1":"add"}`)

	assert.NilError(t, m.mutateNodesAction(context.Background(), oldWs, newWs))
	assert.Equal(t, newWs.Spec.Replica, 1)
	assert.Equal(t, newWs.Spec.NodeFlavor, "flavor1")
}

// TestWorkspaceValidateNodesAction verifies node action validation with empty actions.
func TestWorkspaceValidateNodesAction(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.NilError(t, v.validateNodesAction(context.Background(), &v1.Workspace{}, &v1.Workspace{}, false))
}

// TestWorkspaceValidateScaleDown verifies scale-down validation no-op.
func TestWorkspaceValidateScaleDown(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 1}}
	newWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 2}}
	assert.NilError(t, v.validateScaleDown(context.Background(), newWs, oldWs))
}

// TestWorkspaceValidateVolumeRemoved verifies removed volume validation no-op.
func TestWorkspaceValidateVolumeRemoved(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	assert.NilError(t, v.validateVolumeRemoved(context.Background(), ws, ws))
}

// TestWorkspaceValidateOnCreation verifies create-time validation.
func TestWorkspaceValidateOnCreation(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.NilError(t, v.validateOnCreation(context.Background(), validWorkspace("ws1")))
}

// TestWorkspaceValidateOnUpdate verifies update-time validation.
func TestWorkspaceValidateOnUpdate(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.NilError(t, v.validateOnUpdate(context.Background(), validWorkspace("ws1"), validWorkspace("ws1")))
}

// TestWorkspaceValidatorHandle verifies the workspace validator admission handler.
func TestWorkspaceValidatorHandle(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, validWorkspace("ws1"), nil))
	assert.Assert(t, resp.Allowed)

	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Update, validWorkspace("ws1"), validWorkspace("ws1")))
	assert.Assert(t, resp.Allowed)
}

// TestGetWorkspace verifies workspace retrieval helper.
func TestGetWorkspace(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(validWorkspace("ws1")).Build()

	got, err := getWorkspace(ctx, k8sClient, corev1.NamespaceDefault)
	assert.NilError(t, err)
	assert.Assert(t, got == nil)

	got, err = getWorkspace(ctx, k8sClient, "ws1")
	assert.NilError(t, err)
	assert.Assert(t, got != nil)
}

func newSchemeForWebhookTests(t *testing.T) *runtime.Scheme {
	s := runtime.NewScheme()
	err := v1.AddToScheme(s)
	assert.NilError(t, err)
	return s
}

func TestMutateManagers_AddManager(t *testing.T) {
	ctx := context.TODO()
	scheme := newSchemeForWebhookTests(t)

	user := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "u1",
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		Build()

	m := &WorkspaceMutator{Client: k8sClient}
	oldWs := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{Managers: []string{}},
	}
	newWs := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{Managers: []string{"u1"}},
	}

	err := m.mutateManagers(ctx, oldWs, newWs)
	assert.NilError(t, err)

	updated := &v1.User{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: "u1"}, updated)
	assert.NilError(t, err)

	assert.Equal(t, commonuser.HasWorkspaceRight(updated, "ws1"), true)
	assert.Equal(t, commonuser.HasWorkspaceManagedRight(updated, "ws1"), true)
}

func TestMutateManagers_RemoveManager(t *testing.T) {
	ctx := context.TODO()
	scheme := newSchemeForWebhookTests(t)

	u := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "u1",
		},
	}
	// Pre-assign both access and managed rights to match "already manager" state
	commonuser.AssignWorkspace(u, "ws1")
	commonuser.AssignManagedWorkspace(u, "ws1")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(u).
		Build()

	m := &WorkspaceMutator{Client: k8sClient}
	oldWs := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{Managers: []string{"u1"}},
	}
	newWs := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{Managers: []string{}},
	}

	err := m.mutateManagers(ctx, oldWs, newWs)
	assert.NilError(t, err)

	updated := &v1.User{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: "u1"}, updated)
	assert.NilError(t, err)

	// Managed right should be removed, basic access should remain
	assert.Equal(t, commonuser.HasWorkspaceManagedRight(updated, "ws1"), false)
	assert.Equal(t, commonuser.HasWorkspaceRight(updated, "ws1"), true)
}

func TestMutateManagers_AddManager_UserNotFound(t *testing.T) {
	ctx := context.TODO()
	scheme := newSchemeForWebhookTests(t)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	m := &WorkspaceMutator{Client: k8sClient}
	oldWs := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{Managers: []string{}},
	}
	newWs := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{Managers: []string{"u-not-exists"}},
	}

	err := m.mutateManagers(ctx, oldWs, newWs)
	assert.NilError(t, err)

	// Manager that does not exist should be removed from new workspace spec
	assert.Equal(t, len(newWs.Spec.Managers), 0)
}

func TestMutateWorkloadsOfWorkspace_EnablePreempt(t *testing.T) {
	ctx := context.TODO()
	scheme := newSchemeForWebhookTests(t)

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Labels: map[string]string{
				v1.ClusterIdLabel:   "cluster1",
				v1.WorkspaceIdLabel: "ws1",
			},
			Annotations: map[string]string{
				v1.RetryOnOriginalNodesAnnotation: v1.TrueStr,
			},
		},
		Status: v1.WorkloadStatus{
			Phase: v1.WorkloadRunning,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workload).
		Build()

	m := &WorkspaceMutator{Client: k8sClient}
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: v1.WorkspaceSpec{
			Cluster:       "cluster1",
			EnablePreempt: true,
		},
	}

	err := m.mutateWorkloadsOfWorkspace(ctx, workspace)
	assert.NilError(t, err)

	updated := &v1.Workload{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: "w1"}, updated)
	assert.NilError(t, err)

	// Should set preempt annotation
	assert.Equal(t, v1.GetAnnotation(updated, v1.WorkloadEnablePreemptAnnotation), v1.TrueStr)
	// Should remove sticky nodes annotation
	assert.Equal(t, v1.GetAnnotation(updated, v1.RetryOnOriginalNodesAnnotation), "")
}

func TestMutateWorkloadsOfWorkspace_DisablePreempt(t *testing.T) {
	ctx := context.TODO()
	scheme := newSchemeForWebhookTests(t)

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Labels: map[string]string{
				v1.ClusterIdLabel:   "cluster1",
				v1.WorkspaceIdLabel: "ws1",
			},
			Annotations: map[string]string{
				v1.WorkloadEnablePreemptAnnotation: v1.TrueStr,
			},
		},
		Status: v1.WorkloadStatus{
			Phase: v1.WorkloadRunning,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workload).
		Build()

	m := &WorkspaceMutator{Client: k8sClient}
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: v1.WorkspaceSpec{
			Cluster:       "cluster1",
			EnablePreempt: false,
		},
	}

	err := m.mutateWorkloadsOfWorkspace(ctx, workspace)
	assert.NilError(t, err)

	updated := &v1.Workload{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: "w1"}, updated)
	assert.NilError(t, err)

	// Should remove preempt annotation
	assert.Equal(t, v1.GetAnnotation(updated, v1.WorkloadEnablePreemptAnnotation), "")
}

func TestMutateWorkloadsOfWorkspace_SetTimeout(t *testing.T) {
	ctx := context.TODO()
	scheme := newSchemeForWebhookTests(t)

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Labels: map[string]string{
				v1.ClusterIdLabel:   "cluster1",
				v1.WorkspaceIdLabel: "ws1",
			},
		},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: "PyTorchJob"}, // TrainScope
			Timeout:          nil,
		},
		Status: v1.WorkloadStatus{
			Phase: v1.WorkloadRunning,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workload).
		Build()

	m := &WorkspaceMutator{Client: k8sClient}
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: v1.WorkspaceSpec{
			Cluster: "cluster1",
			MaxRuntime: map[v1.WorkspaceScope]int{
				v1.TrainScope: 2, // 2 hours = 7200 seconds
			},
		},
	}

	err := m.mutateWorkloadsOfWorkspace(ctx, workspace)
	assert.NilError(t, err)

	updated := &v1.Workload{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: "w1"}, updated)
	assert.NilError(t, err)

	// Should set timeout (2 hours = 7200 seconds)
	assert.Assert(t, updated.Spec.Timeout != nil)
	assert.Equal(t, *updated.Spec.Timeout, 7200)
}

// --- merged from workspace2_test.go ---

// TestWorkspaceValidateVolumesImmutable covers volume immutable-field error branches.
func TestWorkspaceValidateVolumesImmutable(t *testing.T) {
	v := &WorkspaceValidator{}
	oldWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc1", Capacity: "100Gi", AccessMode: corev1.ReadWriteMany},
	}}}

	scChanged := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc2", Capacity: "100Gi", AccessMode: corev1.ReadWriteMany},
	}}}
	assert.Assert(t, v.validateVolumes(scChanged, oldWs) != nil)

	capChanged := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc1", Capacity: "200Gi", AccessMode: corev1.ReadWriteMany},
	}}}
	assert.Assert(t, v.validateVolumes(capChanged, oldWs) != nil)

	zeroCap := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 2, Type: v1.PFS, MountPath: "/p", StorageClass: "sc", Capacity: "0"},
	}}}
	assert.Assert(t, v.validateVolumes(zeroCap, nil) != nil)

	badAccess := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 3, Type: v1.PFS, MountPath: "/p", StorageClass: "sc", Capacity: "10Gi", AccessMode: "Bad"},
	}}}
	assert.Assert(t, v.validateVolumes(badAccess, nil) != nil)
}

// TestWorkspaceValidateNodesActionErrors covers node action validation error branches.
func TestWorkspaceValidateNodesActionErrors(t *testing.T) {
	scheme := newScheme(t)

	// node bound elsewhere cannot be added
	bound := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{Workspace: pointer.String("other")},
		// Managed, so the add reaches the ownership check rather than stopping at the
		// onboarding one that now runs ahead of it.
		Status: v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}},
	}
	c1 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bound).Build()
	v1v := &WorkspaceValidator{Client: c1}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(ws, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	assert.Assert(t, v1v.validateNodesAction(context.Background(), ws, &v1.Workspace{}, false) != nil)

	// node not found
	c2 := fake.NewClientBuilder().WithScheme(scheme).Build()
	v2 := &WorkspaceValidator{Client: c2}
	ws2 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(ws2, v1.WorkspaceNodesAction, `{"missing":"add"}`)
	assert.Assert(t, v2.validateNodesAction(context.Background(), ws2, &v1.Workspace{}, false) != nil)

	// cluster mismatch
	wrongCluster := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n2", Labels: map[string]string{v1.ClusterIdLabel: "other"}},
		Status:     v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}},
	}
	c3 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wrongCluster).Build()
	v3 := &WorkspaceValidator{Client: c3}
	ws3 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(ws3, v1.WorkspaceNodesAction, `{"n2":"add"}`)
	assert.Assert(t, v3.validateNodesAction(context.Background(), ws3, &v1.Workspace{}, false) != nil)
}

// A node that has not finished onboarding cannot be handed to a workspace, and admission is
// where the caller finds that out: accepting the request would mean a 200 for a node that
// then simply never joins.
func TestWorkspaceValidateNodesActionRefusesAnUnmanagedNode(t *testing.T) {
	scheme := newScheme(t)

	managing := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Status:     v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaging}},
	}
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(managing).Build()}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(ws, v1.WorkspaceNodesAction, `{"n1":"add"}`)

	err := v.validateNodesAction(context.Background(), ws, &v1.Workspace{}, false)
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(err.Error(), "not managed yet"), err.Error())
	assert.Assert(t, apierrors.IsConflict(err), err.Error())

	// Add only: a node that ended up bound and then lost its managed state is exactly the one
	// that needs releasing, so the remove has to stay possible.
	lost := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n2", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{Workspace: pointer.String("ws1")},
		Status:     v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManagedFailed}},
	}
	v2 := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(lost).Build()}
	ws2 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(ws2, v1.WorkspaceNodesAction, `{"n2":"remove"}`)
	assert.NilError(t, v2.validateNodesAction(context.Background(), ws2, &v1.Workspace{}, false))
}

// Once a request has been accepted, every later update to the Workspace carries the same
// annotation through here -- a replica edit, a volume, a label. Re-judging it rejects those
// unrelated writes for as long as the controller is working through the request, and against
// node state that has moved on since the request was accepted.
func TestWorkspaceValidateNodesActionSkipsAnUnchangedRequest(t *testing.T) {
	scheme := newScheme(t)
	// A node that would fail every check the loop makes: bound elsewhere, wrong cluster, not
	// managed. None of it matters, because nothing new is being asked for.
	stale := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "other"}},
		Spec:       v1.NodeSpec{Workspace: pointer.String("ws-other")},
		Status:     v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManagedFailed}},
	}
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(stale).Build()}
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(oldWs, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	newWs := oldWs.DeepCopy()
	newWs.Spec.Replica = 3

	assert.NilError(t, v.validateNodesAction(context.Background(), newWs, oldWs, false))

	// The same annotation arriving for the first time is still judged.
	assert.Assert(t, v.validateNodesAction(context.Background(), newWs, &v1.Workspace{}, false) != nil)
}

// requestingWorkspace is a workspace with a nodes-action request in flight. The flavor and the
// resource name it carries are what a real one always has by the time a request is accepted,
// and without them mutateByNodeFlavor rewrites Spec.Replica before anything here is reached.
func requestingWorkspace(replica int, actions string) *v1.Workspace {
	w := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: v1.WorkspaceSpec{Cluster: "cluster1", NodeFlavor: "flavor", Replica: replica}}
	v1.SetAnnotation(w, v1.GpuResourceNameAnnotation, "amd.com/gpu")
	if actions != "" {
		v1.SetAnnotation(w, v1.WorkspaceNodesAction, actions)
	}
	return w
}

// A withdrawal is a withdrawal only if it is nothing else besides. Being recognised as one is
// a pass out of mutateCommon and out of both the in-flight and the scale-down checks, so any
// field that rides along on the write is a field that reaches storage unnormalised and
// unvalidated. The predicate has to be able to tell the controller's three-field patch from
// somebody's edit wearing it as a costume.
func TestWorkspaceWithdrawalCannotCarryAnythingElse(t *testing.T) {
	flavor := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "flavor"}}
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	scheme := newScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(flavor, cluster).Build()
	m := &WorkspaceMutator{Client: cli}
	v := &WorkspaceValidator{Client: cli}

	oldWs := requestingWorkspace(3, `{"n1":"add","n2":"add"}`)
	withdrawal := func() *v1.Workspace {
		w := oldWs.DeepCopy()
		w.Spec.Replica = 2
		v1.SetAnnotation(w, v1.WorkspaceNodesAction, `{"n2":"add"}`)
		v1.SetAnnotation(w, v1.WorkspaceNodesActionError, "n1: it is already bound to ws-other")
		return w
	}
	assert.Assert(t, isNodesActionWithdrawal(oldWs, withdrawal()), "the bare shape is one")

	// Each of these is the controller's exact patch with one extra thing attached. None of
	// them is what dropRefusedActions writes, and none of them may buy the exemption.
	volumes := withdrawal()
	volumes.Spec.Volumes = append(volumes.Spec.Volumes, v1.WorkspaceVolume{Type: v1.HOSTPATH, Id: 7})
	managers := withdrawal()
	managers.Spec.Managers = append(managers.Spec.Managers, "someone-else")
	flavorSwap := withdrawal()
	flavorSwap.Spec.NodeFlavor = "another-flavor"
	labelled := withdrawal()
	metav1.SetMetaDataLabel(&labelled.ObjectMeta, v1.DisplayNameLabel, "renamed")
	annotated := withdrawal()
	v1.SetAnnotation(annotated, v1.GpuResourceNameAnnotation, "nvidia.com/gpu")

	for name, smuggled := range map[string]*v1.Workspace{
		"a volume": volumes, "a manager": managers, "a flavour": flavorSwap,
		"a label": labelled, "an annotation": annotated,
	} {
		assert.Assert(t, !isNodesActionWithdrawal(oldWs, smuggled), "%s rode along", name)
		// And the write itself does not get through: no longer a withdrawal, it is a second
		// nodes-action on a request that is already in flight.
		assert.Assert(t, admit(t, m, v, oldWs, smuggled) != nil, "%s was admitted", name)
	}
}

// A forced request withdrawn whole. dropRefusedActions drops WorkspaceForcedAction in the
// same patch when nothing is left of the request, so this write differs from the stored
// object by one annotation more than a partial withdrawal does -- and that extra key is the
// whole test. Unrecognised, the write falls through to mutateNodesAction and is rejected for
// moving Spec.Replica alongside the annotation, which the controller has no way to recover
// from: it re-sends the same patch forever, the request never ends, the replica is never
// given back, and every later nodes-action and scale-down on the workspace queues behind it.
//
// TestWorkspaceWithdrawalCannotCarryAnythingElse covers the opposite direction -- what may
// not ride along -- and cannot catch this one, where the extra key is the controller's own.
func TestWorkspaceAdmitForcedWithdrawalOfTheWholeRequest(t *testing.T) {
	scheme := newScheme(t)
	taken := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{Workspace: pointer.String("ws-other")},
	}
	flavor := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "flavor"}}
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(taken, flavor, cluster).Build()
	m := &WorkspaceMutator{Client: cli}
	v := &WorkspaceValidator{Client: cli}

	// A forced request for the workspace's only in-flight entry, as the apiserver writes it.
	oldWs := validWorkspace("ws1")
	oldWs.Spec.NodeFlavor = "flavor"
	oldWs.Spec.Replica = 3
	v1.SetAnnotation(oldWs, v1.GpuResourceNameAnnotation, "amd.com/gpu")
	v1.SetAnnotation(oldWs, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	v1.SetAnnotation(oldWs, v1.WorkspaceForcedAction, v1.TrueStr)

	// n1 was taken before it could bind, so nothing is left: annotation, forced flag and the
	// replica it was charged all go in one patch.
	newWs := oldWs.DeepCopy()
	newWs.Spec.Replica = 2
	v1.RemoveAnnotation(newWs, v1.WorkspaceNodesAction)
	v1.RemoveAnnotation(newWs, v1.WorkspaceForcedAction)
	v1.SetAnnotation(newWs, v1.WorkspaceNodesActionError, "n1: it is already bound to ws-other")

	assert.Assert(t, isNodesActionWithdrawal(oldWs, newWs), "the forced shape is a withdrawal")
	assert.NilError(t, admit(t, m, v, oldWs, newWs))
	// Untouched by either webhook: the refund the controller wrote, and the reason it wrote
	// it for.
	assert.Equal(t, newWs.Spec.Replica, 2)
	assert.Equal(t, v1.GetAnnotation(newWs, v1.WorkspaceNodesActionError), "n1: it is already bound to ws-other")
	assert.Assert(t, !v1.HasAnnotation(newWs, v1.WorkspaceForcedAction))

	// The exemption only ever goes away with the request. Setting it under cover of the
	// withdrawal shape would buy a remove its way past validateNodesRemoved's in-use check,
	// so a write that turns it on is not a withdrawal however well the rest of it fits.
	partial := requestingWorkspace(3, `{"n1":"add","n2":"add"}`)
	forging := partial.DeepCopy()
	forging.Spec.Replica = 2
	v1.SetAnnotation(forging, v1.WorkspaceNodesAction, `{"n2":"add"}`)
	v1.SetAnnotation(forging, v1.WorkspaceNodesActionError, "n1: it is already bound to ws-other")
	v1.SetAnnotation(forging, v1.WorkspaceForcedAction, v1.TrueStr)
	assert.Assert(t, !isNodesActionWithdrawal(partial, forging), "the forced flag was switched on")
}

// The controller giving up on an entry it cannot bind. Its patch carries both the shrunk
// request and the Spec.Replica those entries were charged; the mutator's only job is to keep
// its hands off it, because anything it changed here the validator would see instead of what
// the controller actually wrote.
func TestWorkspaceMutateLeavesAWithdrawalAlone(t *testing.T) {
	flavor := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "flavor"}}
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(newScheme(t)).
		WithObjects(flavor).Build()}
	oldWs := requestingWorkspace(3, `{"n1":"add","n2":"add"}`)

	// One entry withdrawn out of two: the writer has already given the replica back.
	newWs := oldWs.DeepCopy()
	newWs.Spec.Replica = 2
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"n2":"add"}`)
	v1.SetAnnotation(newWs, v1.WorkspaceNodesActionError, "n1: it is already bound to ws-other")
	assert.NilError(t, m.mutateOnUpdate(context.Background(), oldWs, newWs))
	assert.Equal(t, newWs.Spec.Replica, 2)
	// The entry left standing keeps its charge -- the forward pass counted it once when the
	// request was accepted and must not run over it again -- and the reason survives, which
	// mutateNodesAction would have wiped had the write reached it.
	assert.Equal(t, v1.GetWorkspaceNodesAction(newWs), `{"n2":"add"}`)
	assert.Equal(t, v1.GetAnnotation(newWs, v1.WorkspaceNodesActionError),
		"n1: it is already bound to ws-other")

	// The whole request withdrawn at once, annotation and all.
	newWs = oldWs.DeepCopy()
	newWs.Spec.Replica = 1
	v1.RemoveAnnotation(newWs, v1.WorkspaceNodesAction)
	v1.SetAnnotation(newWs, v1.WorkspaceNodesActionError, "n1: gone; n2: gone")
	assert.NilError(t, m.mutateOnUpdate(context.Background(), oldWs, newWs))
	assert.Equal(t, newWs.Spec.Replica, 1)
}

// A refund is worth a machine, so the shape that earns one has to be narrow. Anything that is
// not the controller taking its own entries back goes down the ordinary path, where a request
// being edited underneath the controller is turned away by the in-flight check.
func TestWorkspaceWithdrawnNodesActionIsNotJustAnySmallerRequest(t *testing.T) {
	base := requestingWorkspace(3, `{"n1":"add","n2":"add"}`)
	withdrawal := func() *v1.Workspace {
		w := base.DeepCopy()
		w.Spec.Replica = 2
		v1.SetAnnotation(w, v1.WorkspaceNodesAction, `{"n2":"add"}`)
		v1.SetAnnotation(w, v1.WorkspaceNodesActionError, "n1: it is already bound to ws-other")
		return w
	}
	assert.Assert(t, isNodesActionWithdrawal(base, withdrawal()), "the shape itself is a withdrawal")

	// And still one when the reason it writes is the text already there. The same node
	// refused twice for the same cause produces the same sentence; requiring the reason to
	// have changed would leave those entries in flight with no write able to take them out.
	repeated := withdrawal()
	v1.SetAnnotation(base, v1.WorkspaceNodesActionError,
		v1.GetAnnotation(repeated, v1.WorkspaceNodesActionError))
	assert.Assert(t, isNodesActionWithdrawal(base, repeated), "a repeated reason is still a reason")
	v1.RemoveAnnotation(base, v1.WorkspaceNodesActionError)

	cases := map[string]func(*v1.Workspace){
		"no reason means nobody withdrew anything": func(w *v1.Workspace) {
			v1.RemoveAnnotation(w, v1.WorkspaceNodesActionError)
		},
		// A reason already on the object buys nothing on its own: what makes this a withdrawal
		// is entries leaving, and an unrelated write that merely carries the reason along
		// takes none out.
		"a stale reason carried through an unrelated write is not a fresh one": func(w *v1.Workspace) {
			v1.SetAnnotation(base, v1.WorkspaceNodesActionError,
				v1.GetAnnotation(w, v1.WorkspaceNodesActionError))
			v1.SetAnnotation(w, v1.WorkspaceNodesAction, v1.GetWorkspaceNodesAction(base))
			w.Spec.Replica = base.Spec.Replica
		},
		// The refund is an exact number, not a direction. Accepting any smaller replica would
		// hand a forged withdrawal a scale-down of its author's choosing, and one that skips
		// validateScaleDown at that.
		"the replica may not be taken further down than the withdrawal earns": func(w *v1.Workspace) {
			w.Spec.Replica = 1
		},
		"the replica may not be left standing either": func(w *v1.Workspace) { w.Spec.Replica = 3 },
		"the replica may not go up":                   func(w *v1.Workspace) { w.Spec.Replica = 4 },
		"an entry may not change value":               func(w *v1.Workspace) { v1.SetAnnotation(w, v1.WorkspaceNodesAction, `{"n2":"remove"}`) },
		"an entry may not arrive":                     func(w *v1.Workspace) { v1.SetAnnotation(w, v1.WorkspaceNodesAction, `{"n2":"add","n3":"add"}`) },
		"unreadable is not a subset of anything":      func(w *v1.Workspace) { v1.SetAnnotation(w, v1.WorkspaceNodesAction, `{`) },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			original := base.DeepCopy()
			defer func() { base = original }()
			w := withdrawal()
			breakIt(w)
			assert.Assert(t, !isNodesActionWithdrawal(base, w))
		})
	}
}

// The validating webhook sees the withdrawal patch too, and its in-flight check reads a
// shrinking request as a second author editing the first one's work. Rejecting it there would
// stop the withdrawal just as surely as the mutator would, and for the same reason: this is the
// controller ending a request, not somebody else changing it.
func TestWorkspaceValidateNodesActionAllowsAWithdrawal(t *testing.T) {
	scheme := newScheme(t)
	taken := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{Workspace: pointer.String("ws-other")},
	}
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(taken).Build()}
	oldWs := requestingWorkspace(2, `{"n1":"add","n2":"add"}`)
	newWs := oldWs.DeepCopy()
	newWs.Spec.Replica = 1
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"n2":"add"}`)
	v1.SetAnnotation(newWs, v1.WorkspaceNodesActionError, "n1: it is already bound to ws-other")

	// The real predicate, not a hand-passed flag: validateOnUpdate is what decides this, and a
	// test that asserted the exemption while asserting nothing about what triggers it would
	// pass just as happily for a shape that is not a withdrawal at all.
	assert.Assert(t, isNodesActionWithdrawal(oldWs, newWs), "the shape is a withdrawal")
	assert.NilError(t, v.validateNodesAction(context.Background(), newWs, oldWs,
		isNodesActionWithdrawal(oldWs, newWs)))

	// The same shrinking request without a reason on it is somebody editing a request that is
	// already being worked on, and is still turned away.
	edited := oldWs.DeepCopy()
	v1.SetAnnotation(edited, v1.WorkspaceNodesAction, `{"n2":"add"}`)
	assert.Assert(t, !isNodesActionWithdrawal(oldWs, edited))
	assert.Assert(t, v.validateNodesAction(context.Background(), edited, oldWs,
		isNodesActionWithdrawal(oldWs, edited)) != nil)
}

// admit runs the two webhooks the way the API server does: mutating first, and validating
// against whatever the mutator left behind rather than against what the client sent.
//
// Testing them apart cannot see the class of bug this exists for. The withdrawal protocol is
// recognised by the shape of a write, and a mutator that alters any part of that shape leaves
// the validator judging a different write than the one the controller made -- each webhook
// correct on its own, the pair of them wrong.
func admit(t *testing.T, m *WorkspaceMutator, v *WorkspaceValidator, stored, incoming *v1.Workspace) error {
	t.Helper()
	if err := m.mutateOnUpdate(context.Background(), stored, incoming); err != nil {
		return err
	}
	return v.validateOnUpdate(context.Background(), incoming, stored)
}

// A withdrawal has to survive both webhooks in sequence. It is the only write that ends a
// request the controller cannot carry out, so anything that turns it away leaves the workspace
// holding a request forever, with the entry it cannot bind still counted in Spec.Replica.
func TestWorkspaceAdmitWithdrawalEndToEnd(t *testing.T) {
	scheme := newScheme(t)
	taken := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{Workspace: pointer.String("ws-other")},
	}
	free := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2", Labels: map[string]string{
		v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavor",
	}}}
	flavor := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "flavor"}}
	// Sourced from a workload that has not finished, which is what validateScaleDown is there
	// to protect. A withdrawal lowers Spec.Replica and would trip it, but the capacity being
	// given back was never bound to anything for the workload to be running on.
	running := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Namespace: "ws1"}}
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}

	build := func() (*WorkspaceMutator, *WorkspaceValidator) {
		cli := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(taken, free, flavor, running, cluster).Build()
		return &WorkspaceMutator{Client: cli}, &WorkspaceValidator{Client: cli}
	}
	stored := func(sourced bool) *v1.Workspace {
		w := validWorkspace("ws1")
		w.Spec.NodeFlavor = "flavor"
		w.Spec.Replica = 3
		v1.SetAnnotation(w, v1.GpuResourceNameAnnotation, "amd.com/gpu")
		v1.SetAnnotation(w, v1.WorkspaceNodesAction, `{"n1":"add","n2":"add"}`)
		if sourced {
			v1.SetLabel(w, v1.SourceWorkloadIdLabel, "wl1")
		}
		return w
	}

	t.Run("one entry withdrawn out of two", func(t *testing.T) {
		m, v := build()
		old := stored(false)
		w := old.DeepCopy()
		w.Spec.Replica = 2
		v1.SetAnnotation(w, v1.WorkspaceNodesAction, `{"n2":"add"}`)
		v1.SetAnnotation(w, v1.WorkspaceNodesActionError, "n1: it is already bound to ws-other")
		assert.NilError(t, admit(t, m, v, old, w))
		// Both halves of the patch reach the far side untouched.
		assert.Equal(t, w.Spec.Replica, 2)
		assert.Equal(t, v1.GetWorkspaceNodesAction(w), `{"n2":"add"}`)
		assert.Equal(t, v1.GetAnnotation(w, v1.WorkspaceNodesActionError),
			"n1: it is already bound to ws-other")
	})

	t.Run("the whole request withdrawn", func(t *testing.T) {
		m, v := build()
		old := stored(false)
		w := old.DeepCopy()
		w.Spec.Replica = 1
		v1.RemoveAnnotation(w, v1.WorkspaceNodesAction)
		v1.SetAnnotation(w, v1.WorkspaceNodesActionError, "n1: taken; n2: taken")
		assert.NilError(t, admit(t, m, v, old, w))
		assert.Equal(t, w.Spec.Replica, 1)
	})

	t.Run("a workspace sourced from a running workload", func(t *testing.T) {
		m, v := build()
		old := stored(true)
		w := old.DeepCopy()
		w.Spec.Replica = 2
		v1.SetAnnotation(w, v1.WorkspaceNodesAction, `{"n2":"add"}`)
		v1.SetAnnotation(w, v1.WorkspaceNodesActionError, "n1: it is already bound to ws-other")
		assert.NilError(t, admit(t, m, v, old, w))
		assert.Equal(t, w.Spec.Replica, 2)
	})

	// The exemptions above are for the controller ending its own request, and nothing else may
	// reach them. Each of these is a client write that looks like a withdrawal from one angle.
	t.Run("an author shrinking their own in-flight request", func(t *testing.T) {
		m, v := build()
		old := stored(false)
		w := old.DeepCopy()
		v1.SetAnnotation(w, v1.WorkspaceNodesAction, `{"n2":"add"}`)
		assert.Assert(t, admit(t, m, v, old, w) != nil)
	})
	t.Run("a reason annotation used to smuggle a scale-down", func(t *testing.T) {
		m, v := build()
		old := stored(true)
		w := old.DeepCopy()
		w.Spec.Replica = 0
		v1.SetAnnotation(w, v1.WorkspaceNodesAction, `{"n2":"add"}`)
		v1.SetAnnotation(w, v1.WorkspaceNodesActionError, "please just let me through")
		assert.Assert(t, admit(t, m, v, old, w) != nil)
	})
}

// A node already bound elsewhere is refused by the mutating webhook, before any of it is
// counted into Spec.Replica. The validator refuses it too, but a mutator that leans on the
// validator to undo its own arithmetic is one misconfigured webhook away from persisting a
// replica count nobody asked for.
func TestWorkspaceMutateNodesActionRefusesABoundNode(t *testing.T) {
	scheme := newScheme(t)
	// Managed, and bound. Without the phase the not-managed guard above intercepts the add
	// and this test passes on the strength of a check it is not about -- delete the bound-node
	// guard and it would stay green.
	taken := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavor",
		}},
		Spec:   v1.NodeSpec{Workspace: pointer.String("ws-other")},
		Status: v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}},
	}
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(taken).Build()}

	oldWs := requestingWorkspace(1, "")
	newWs := requestingWorkspace(1, `{"n1":"add"}`)
	err := m.mutateNodesAction(context.Background(), oldWs, newWs)
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(err.Error(), "is bound for ws-other"), err)
	assert.Equal(t, newWs.Spec.Replica, 1, "a refused entry must not be charged")

	// The mirror of it: releasing somebody else's node would decrement a count that was never
	// counting it.
	newWs = requestingWorkspace(1, `{"n1":"remove"}`)
	err = m.mutateNodesAction(context.Background(), oldWs, newWs)
	assert.Assert(t, err != nil)
	assert.Equal(t, newWs.Spec.Replica, 1)
}

// Which flavor a nodes-action settles on, and which entry gets blamed when they disagree.
func TestWorkspaceMutateNodesActionFlavor(t *testing.T) {
	scheme := newScheme(t)
	node := func(name, flavor string) *v1.Node {
		return &v1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{
				v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: flavor,
			}},
			Status: v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}},
		}
	}
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(node("a1", "flavor"), node("b2", "other"), node("c3", "other")).Build()}

	// A workspace holding nothing may be handed a different flavor than the one left over
	// from the nodes it used to have. Scaling to zero does not clear Spec.NodeFlavor, so
	// checking the add against it would leave the workspace pinned to a flavor that no longer
	// describes anything it owns, with no way out short of deleting it.
	emptied := requestingWorkspace(0, `{"b2":"add"}`)
	assert.NilError(t, m.mutateNodesAction(context.Background(), requestingWorkspace(0, ""), emptied))
	assert.Equal(t, emptied.Spec.NodeFlavor, "other")
	assert.Equal(t, emptied.Spec.Replica, 1)

	// A workspace that still holds nodes does not get to change flavor this way.
	held := requestingWorkspace(1, `{"b2":"add"}`)
	err := m.mutateNodesAction(context.Background(), requestingWorkspace(1, ""), held)
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(err.Error(), "flavor(other)"), err)

	// Two adds that disagree, on an empty workspace: the first in key order settles it and
	// the second is the one refused -- the same one on every run, which is the whole reason
	// the keys are sorted rather than ranged over.
	for range 8 {
		mixed := requestingWorkspace(0, `{"a1":"add","c3":"add"}`)
		mixed.Spec.NodeFlavor = ""
		err = m.mutateNodesAction(context.Background(), requestingWorkspace(0, ""), mixed)
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "flavor(other)"), err)
	}
}

// A node that has not finished being taken into the cluster cannot be added, and is refused
// before the cluster check rather than after it: an unmanaged node has no cluster label yet,
// so the cluster check would turn it away reporting a mismatch that does not exist and the
// error would send whoever asked looking in the wrong place entirely.
//
// Refused in the mutator, like the bound-node case beside it, because everything past this
// point moves Spec.Replica.
func TestWorkspaceMutateNodesActionRefusesAnUnmanagedNode(t *testing.T) {
	scheme := newScheme(t)
	// No cluster label and no managed phase -- a node that has been registered and nothing
	// more, which is exactly the state that makes the ordering matter.
	fresh := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
		v1.NodeFlavorIdLabel: "flavor",
	}}}
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(fresh).Build()}

	newWs := requestingWorkspace(1, `{"n1":"add"}`)
	err := m.mutateNodesAction(context.Background(), requestingWorkspace(1, ""), newWs)
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(err.Error(), "is not managed yet"), err)
	assert.Equal(t, newWs.Spec.Replica, 1, "a refused entry must not be charged")

	// Removes are not held to it. A node being taken back out of the cluster stops being
	// managed while a workspace still holds it, and that release has to be able to happen.
	held := fresh.DeepCopy()
	held.Spec.Workspace = pointer.String("ws1")
	m = &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(held).Build()}
	newWs = requestingWorkspace(1, `{"n1":"remove"}`)
	err = m.mutateNodesAction(context.Background(), requestingWorkspace(1, ""), newWs)
	assert.Assert(t, err != nil)
	assert.Assert(t, !strings.Contains(err.Error(), "is not managed yet"), err)
}

// Scale-down is refused while a nodes-action is in flight, by the validator as well as by the
// mutator. Two copies of one rule, and each needs its own pin: they are reached by different
// paths -- validateOnUpdate routes withdrawals away from this one and not from that one -- so
// a test through the mutator says nothing about whether this copy still exists.
//
// It reads the old object on purpose. mutateScaleDown may have written a scale-down request
// onto the new one by the time this runs, and the question is what the request arrived
// against.
func TestWorkspaceValidateScaleDownRefusesWhileANodesActionIsInFlight(t *testing.T) {
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(newScheme(t)).Build()}
	oldWs := requestingWorkspace(3, `{"n1":"add"}`)
	newWs := requestingWorkspace(2, `{"n1":"add"}`)

	err := v.validateScaleDown(context.Background(), newWs, oldWs)
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(err.Error(), "is processing"), err)

	// Nothing in flight, nothing to wait for.
	assert.NilError(t, v.validateScaleDown(context.Background(),
		requestingWorkspace(2, ""), requestingWorkspace(3, "")))
	// Not a scale-down at all: the count is going up, and the annotation is beside the point.
	assert.NilError(t, v.validateScaleDown(context.Background(), requestingWorkspace(4, `{"n1":"add"}`), oldWs))
}

// Removes may not take Spec.Replica below zero. Reachable only when the count is already
// behind what the workspace holds -- each entry was checked against the node's own claim, so
// a remove that gets this far is a node this workspace really has -- and a negative replica
// is a number no part of the system further down knows how to read.
func TestWorkspaceMutateNodesActionCannotDriveReplicaNegative(t *testing.T) {
	scheme := newScheme(t)
	mine := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavor",
		}},
		Spec:   v1.NodeSpec{Workspace: pointer.String("ws1")},
		Status: v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}},
	}
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(mine).Build()}

	// The count says zero while a node is still claimed: the remove is accepted, because the
	// claim is real, and the arithmetic would land on -1.
	newWs := requestingWorkspace(0, `{"n1":"remove"}`)
	assert.NilError(t, m.mutateNodesAction(context.Background(), requestingWorkspace(0, ""), newWs))
	assert.Equal(t, newWs.Spec.Replica, 0)
}

// The reason is cleared by the next request being accepted, and only by that. A request going
// away is not an acceptance -- the controller clears the annotation immediately after writing
// the reason a part of it was dropped, and wiping it there would leave whoever asked with a
// request that vanished and nothing saying why.
func TestWorkspaceMutateNodesActionClearsTheReasonOnlyForANewRequest(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n9", Labels: map[string]string{
		v1.NodeFlavorIdLabel: "flavor", v1.ClusterIdLabel: "cluster1",
	}}, Status: v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}}}
	flavor := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "flavor"}}
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(node, flavor).Build()}
	oldWs := requestingWorkspace(1, "")
	v1.SetAnnotation(oldWs, v1.WorkspaceNodesActionError, "n1: it is already bound to ws-other")

	cleared := oldWs.DeepCopy()
	v1.SetAnnotation(cleared, v1.WorkspaceNodesAction, `{"n9":"add"}`)
	assert.NilError(t, m.mutateNodesAction(context.Background(), oldWs, cleared))
	assert.Equal(t, v1.GetAnnotation(cleared, v1.WorkspaceNodesActionError), "")

	kept := oldWs.DeepCopy()
	assert.NilError(t, m.mutateNodesAction(context.Background(), oldWs, kept))
	assert.Assert(t, v1.GetAnnotation(kept, v1.WorkspaceNodesActionError) != "")
}

// TestWorkspaceMutateNodesActionErrors covers node action mutation error branches.
func TestWorkspaceMutateNodesActionErrors(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	// replica change with action set -> error
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", Replica: 1}}
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", Replica: 2}}
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	assert.Assert(t, m.mutateNodesAction(context.Background(), oldWs, newWs) != nil)

	// node not found -> error
	oldWs2 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	newWs2 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(newWs2, v1.WorkspaceNodesAction, `{"missing":"add"}`)
	assert.Assert(t, m.mutateNodesAction(context.Background(), oldWs2, newWs2) != nil)

	// flavor mismatch -> error
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavorX",
		}},
		Spec: v1.NodeSpec{Workspace: pointer.String("")},
	}
	mc := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()}
	oldWs3 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", NodeFlavor: "flavor1", Replica: 1}}
	newWs3 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", NodeFlavor: "flavor1", Replica: 1}}
	v1.SetAnnotation(newWs3, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	assert.Assert(t, mc.mutateNodesAction(context.Background(), oldWs3, newWs3) != nil)
}

// TestWorkspaceMutateManagersRemove covers manager removal mutation.
func TestWorkspaceMutateManagersRemove(t *testing.T) {
	scheme := newScheme(t)
	u := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}}
	commonuser.AssignWorkspace(u, "ws1")
	commonuser.AssignManagedWorkspace(u, "ws1")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(u).Build()
	m := &WorkspaceMutator{Client: k8sClient}

	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Managers: []string{"u1"}}}
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Managers: []string{}}}
	assert.NilError(t, m.mutateManagers(context.Background(), oldWs, newWs))

	updated := &v1.User{}
	assert.NilError(t, k8sClient.Get(context.Background(), client.ObjectKey{Name: "u1"}, updated))
	assert.Assert(t, !commonuser.HasWorkspaceManagedRight(updated, "ws1"))
}

// TestWorkspaceValidateRequiredParamsBranches covers required-param error branches.
func TestWorkspaceValidateRequiredParamsBranches(t *testing.T) {
	v := &WorkspaceValidator{}
	// bad queue policy + empty displayName
	w := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{
		Cluster:     "cluster1",
		QueuePolicy: "bad",
	}}
	v1.SetLabel(w, v1.ClusterIdLabel, "cluster1")
	assert.Assert(t, v.validateRequiredParams(w) != nil)
}

// --- merged from workspace3_test.go ---

// TestWorkspaceMutateManagersUserNotFound covers removal of non-existent managers.
func TestWorkspaceMutateManagersUserNotFound(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Managers: []string{"missing"}}}
	assert.NilError(t, m.mutateManagers(context.Background(), nil, newWs))
	assert.Equal(t, len(newWs.Spec.Managers), 0)
}

// TestWorkspaceMutateGpuProductError covers the flavor-not-found error branch.
func TestWorkspaceMutateGpuProductError(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{NodeFlavor: "missing"}}
	assert.Assert(t, m.mutateGpuProduct(context.Background(), ws) != nil)
}

// TestWorkspaceValidateRelatedResourceClusterMissing covers cluster-not-found branch.
func TestWorkspaceValidateRelatedResourceClusterMissing(t *testing.T) {
	scheme := newScheme(t)
	nf := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "flavor1"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nf).Build()
	v := &WorkspaceValidator{Client: c}
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 1, NodeFlavor: "flavor1", Cluster: "missing"}}
	assert.Assert(t, v.validateRelatedResource(context.Background(), ws) != nil)
}

// TestWorkspaceValidateNodesActionProcessing covers the concurrent-job processing branch.
func TestWorkspaceValidateNodesActionProcessing(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(oldWs, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"n2":"add"}`)
	assert.Assert(t, v.validateNodesAction(context.Background(), newWs, oldWs, false) != nil)
}

// TestWorkspaceValidatorHandleBranches covers validator decode/deletion/update branches.
func TestWorkspaceValidatorHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}

	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)

	now := metav1.Now()
	deleting := validWorkspace("ws1")
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{"x"}
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Update, deleting, deleting))
	assert.Assert(t, resp.Allowed)
}

// TestWorkspaceMutatorHandleUpdateNodesAction covers update routing into node action mutation.
func TestWorkspaceMutatorHandleUpdateNodesAction(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavor1",
		}},
		Spec:   v1.NodeSpec{Workspace: pointer.String("")},
		Status: v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	m := &WorkspaceMutator{Client: c, decoder: newDecoder(t)}

	oldWs := validWorkspace("ws1")
	newWs := validWorkspace("ws1")
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, newWs, oldWs))
	assert.Assert(t, resp.Allowed)
}

// TestWorkspaceValidateVolumesSelectorChanged covers the selector immutability branch.
func TestWorkspaceValidateVolumesSelectorChanged(t *testing.T) {
	v := &WorkspaceValidator{}
	oldWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/p", Capacity: "10Gi", Selector: map[string]string{"a": "b"}, AccessMode: corev1.ReadWriteMany},
	}}}
	newWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/p", Capacity: "10Gi", Selector: map[string]string{"c": "d"}, AccessMode: corev1.ReadWriteMany},
	}}}
	assert.Assert(t, v.validateVolumes(newWs, oldWs) != nil)
}

// TestGetWorkspaceError covers the workspace retrieval error path.
func TestGetWorkspaceError(t *testing.T) {
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	_, err := getWorkspace(context.Background(), c, "missing")
	assert.Assert(t, err != nil)
}

// TestWorkspaceMutateScaleDownNoop covers the no-op scale-down branches.
func TestWorkspaceMutateScaleDownNoop(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	// newCount >= currentReplica -> nil
	oldWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 3}}
	oldWs.Status.AvailableReplica = 1
	newWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 2}}
	assert.NilError(t, m.mutateScaleDown(context.Background(), oldWs, newWs))
}

// --- merged from workspace4_test.go ---

// TestWorkspaceValidateCommonStepErrors covers validateCommon return-error branches.
func TestWorkspaceValidateCommonStepErrors(t *testing.T) {
	ctx := context.Background()
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	// required params error
	assert.Assert(t, v.validateCommon(ctx, &v1.Workspace{}, nil) != nil)

	// bad dns display name
	dns := validWorkspace("ws1")
	v1.SetLabel(dns, v1.DisplayNameLabel, "Bad.Name")
	assert.Assert(t, v.validateCommon(ctx, dns, nil) != nil)

	// related resource missing flavor (replica increase)
	related := validWorkspace("ws1")
	related.Spec.NodeFlavor = "missing"
	related.Spec.Replica = 2
	assert.Assert(t, v.validateCommon(ctx, related, nil) != nil)
}

// TestWorkspaceValidateOnUpdateStepErrors covers validateOnUpdate return-error branches.
func TestWorkspaceValidateOnUpdateStepErrors(t *testing.T) {
	ctx := context.Background()
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	// immutable cluster change
	oldWs := validWorkspace("ws1")
	clusterChanged := validWorkspace("ws1")
	clusterChanged.Spec.Cluster = "cluster2"
	assert.Assert(t, v.validateOnUpdate(ctx, clusterChanged, oldWs) != nil)

	// nodes action references missing node
	nodesAction := validWorkspace("ws1")
	v1.SetAnnotation(nodesAction, v1.WorkspaceNodesAction, `{"missing":"add"}`)
	assert.Assert(t, v.validateOnUpdate(ctx, nodesAction, oldWs) != nil)
}

// TestWorkspaceValidateOnUpdateScaleDown covers the scale-down source workload branch.
func TestWorkspaceValidateOnUpdateScaleDown(t *testing.T) {
	scheme := newScheme(t)
	src := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "src"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(src).Build()
	v := &WorkspaceValidator{Client: c}

	oldWs := validWorkspace("ws1")
	oldWs.Spec.Replica = 2
	newWs := validWorkspace("ws1")
	newWs.Spec.Replica = 1
	v1.SetLabel(newWs, v1.SourceWorkloadIdLabel, "src")
	assert.Assert(t, v.validateOnUpdate(context.Background(), newWs, oldWs) != nil)
}

// TestWorkspaceMutateOnUpdatePreempt covers preempt-driven workload mutation routing.
func TestWorkspaceMutateOnUpdatePreempt(t *testing.T) {
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &WorkspaceMutator{Client: c}

	oldWs := validWorkspace("ws1")
	newWs := validWorkspace("ws1")
	newWs.Spec.EnablePreempt = true
	assert.NilError(t, m.mutateOnUpdate(context.Background(), oldWs, newWs))
}

// TestWorkspaceMutatorHandleFullCreate covers the mutator create marshal/patch path.
func TestWorkspaceMutatorHandleFullCreate(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	m := &WorkspaceMutator{Client: c, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, validWorkspace("ws1"), nil))
	assert.Assert(t, resp.Allowed)
}

// A scale-down is a new nodes-action request, and a new request supersedes whatever the last
// one failed with -- the same lifecycle mutateNodesAction gives an explicit one. Nothing else
// clears the reason: without this, an add turned down once stays on display through every
// unrelated operation that follows, and every operator and UI reading the annotation reports
// a binding failure that is not happening.
func TestWorkspaceMutateScaleDownClearsAStaleReason(t *testing.T) {
	scheme := newScheme(t)
	held := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavor",
			v1.WorkspaceIdLabel: "ws1",
		}},
		Spec: v1.NodeSpec{Workspace: pointer.String("ws1")},
	}
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(held).Build()}

	oldWs := requestingWorkspace(1, "")
	oldWs.Status.AvailableReplica = 1
	v1.SetAnnotation(oldWs, v1.WorkspaceNodesActionError, "n9: it is already bound to ws-other")
	newWs := requestingWorkspace(0, "")
	v1.SetAnnotation(newWs, v1.WorkspaceNodesActionError, "n9: it is already bound to ws-other")

	assert.NilError(t, m.mutateScaleDown(context.Background(), oldWs, newWs))
	assert.Equal(t, v1.GetWorkspaceNodesAction(newWs), `{"n1":"remove"}`)
	assert.Equal(t, v1.GetAnnotation(newWs, v1.WorkspaceNodesActionError), "",
		"the reason belongs to the request this one replaces")
}

// The other half of it: a scale-down that cannot be built must not clear anything, and must
// say what is actually wrong. A node the workspace is counted as holding but no longer has
// the claim on is not one it can release, so the candidate list comes up short -- and building
// the request anyway would put a node that is not short in its place and release a machine
// nobody asked to give back.
func TestWorkspaceMutateScaleDownRefusesAShortCandidateList(t *testing.T) {
	scheme := newScheme(t)
	// Labelled ws1, claimed by ws2: mid-flight, and not ws1's to release.
	stolen := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavor",
			v1.WorkspaceIdLabel: "ws1",
		}},
		Spec: v1.NodeSpec{Workspace: pointer.String("ws2")},
	}
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(stolen).Build()}

	oldWs := requestingWorkspace(1, "")
	oldWs.Status.AvailableReplica = 1
	v1.SetAnnotation(oldWs, v1.WorkspaceNodesActionError, "n9: it is already bound to ws-other")
	newWs := requestingWorkspace(0, "")
	v1.SetAnnotation(newWs, v1.WorkspaceNodesActionError, "n9: it is already bound to ws-other")

	err := m.mutateScaleDown(context.Background(), oldWs, newWs)
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(err.Error(), "free to release"), err)
	assert.Equal(t, v1.GetWorkspaceNodesAction(newWs), "", "no request was built")
	assert.Assert(t, v1.GetAnnotation(newWs, v1.WorkspaceNodesActionError) != "",
		"nothing was superseded, so nothing is cleared")
}

// mutateOnUpdate decides whether a write is a withdrawal before it runs any mutation, and this
// is the case that tells the difference. Spec.NodeFlavor is not immutable, so a workspace can
// arrive here with the field cleared -- and mutateByNodeFlavor answers a cleared flavor by
// zeroing Spec.Replica, which is one of the fields the withdrawal's shape is made of.
//
// Deciding after that mutation ran would read a replica the controller never sent, fail the
// shape check, and hand the write to mutateNodesAction: the reason annotation stripped, the
// surviving entry charged to Spec.Replica a second time, and then the validator rejecting what
// is left. Deciding first, the mutator returns before mutateByNodeFlavor is reached at all.
func TestWorkspaceAdmitWithdrawalBeforeAnyMutation(t *testing.T) {
	scheme := newScheme(t)
	taken := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{Workspace: pointer.String("ws-other")},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(taken).Build()
	m, v := &WorkspaceMutator{Client: cli}, &WorkspaceValidator{Client: cli}

	// Already defaulted, the way anything the controller patches is: mutateCommon has run
	// over it before, which is why a withdrawal can skip it on the way back through.
	stored := validWorkspace("ws1")
	stored.Spec.Replica = 2
	// Cleared -- not immutable, and the field mutateByNodeFlavor answers by zeroing Replica.
	stored.Spec.NodeFlavor = ""
	v1.SetAnnotation(stored, v1.WorkspaceNodesAction, `{"n1":"add","n2":"add"}`)
	withdrawal := stored.DeepCopy()
	withdrawal.Spec.Replica = 1
	v1.SetAnnotation(withdrawal, v1.WorkspaceNodesAction, `{"n2":"add"}`)
	v1.SetAnnotation(withdrawal, v1.WorkspaceNodesActionError, "n1: it is already bound to ws-other")

	assert.NilError(t, admit(t, m, v, stored, withdrawal))
	// Untouched, all three of them: the refund the controller wrote, the entry it kept, and
	// the reason it recorded. A zeroed replica here is mutateByNodeFlavor having run.
	assert.Equal(t, withdrawal.Spec.Replica, 1)
	assert.Equal(t, v1.GetWorkspaceNodesAction(withdrawal), `{"n2":"add"}`)
	assert.Assert(t, v1.GetAnnotation(withdrawal, v1.WorkspaceNodesActionError) != "")
}

// The replica arithmetic used to run inside the loop that judges the entries, reading
// Spec.Replica as it went, so a request holding both an add and a remove landed somewhere
// different depending on which one Go's map range happened to visit first. From zero the add
// set the count to one and the remove did nothing, so the pair came out as 1 or as 0.
//
// Run the same request enough times that a random order would have shown both answers, and
// assert the arithmetic one: one node in, one node out, net zero.
func TestWorkspaceMutateNodesActionIsOrderIndependent(t *testing.T) {
	scheme := newScheme(t)
	free := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n-add", Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavor1",
		}},
		Status: v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}},
	}
	held := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n-drop", Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavor1",
		}},
		Spec:   v1.NodeSpec{Workspace: pointer.String("ws1")},
		Status: v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}},
	}
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(free, held).Build()}

	for i := 0; i < 64; i++ {
		oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
			Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
		newWs := oldWs.DeepCopy()
		v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"n-add":"add","n-drop":"remove"}`)

		assert.NilError(t, m.mutateNodesAction(context.Background(), oldWs, newWs))
		assert.Equal(t, newWs.Spec.Replica, 0)
		// The flavor still gets settled by the add, which is the half of the old zero-replica
		// branch that was doing real work.
		assert.Equal(t, newWs.Spec.NodeFlavor, "flavor1")
	}
}

// Two adds that disagree about flavor are a rejection either way; which one is named in the
// message is what used to depend on map order. The lower key settles the flavor, so the higher
// one is always the mismatch reported.
func TestWorkspaceMutateNodesActionReportsAStableFlavorMismatch(t *testing.T) {
	scheme := newScheme(t)
	node := func(name, flavor string) *v1.Node {
		return &v1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{
				v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: flavor,
			}},
			Status: v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}},
		}
	}
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(node("a-node", "flavor-a"), node("b-node", "flavor-b")).Build()}

	for i := 0; i < 64; i++ {
		oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
			Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
		newWs := oldWs.DeepCopy()
		v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"a-node":"add","b-node":"add"}`)

		err := m.mutateNodesAction(context.Background(), oldWs, newWs)
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "flavor-b"), err.Error())
	}
}

func inFlightNode(t *testing.T, name, workspace string) *v1.Node {
	t.Helper()
	n := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavor1",
		}},
		Status: v1.NodeStatus{ClusterStatus: v1.NodeClusterStatus{Phase: v1.NodeManaged}},
	}
	if workspace != "" {
		n.Spec.Workspace = pointer.String(workspace)
	}
	return n
}

// A second nodes-action over the top of one still in flight is refused, including -- above
// all -- when the mutator's own skips shrink it onto exactly the request already in flight.
//
// That case used to be admitted as a no-op, because the sets compared equal and the
// annotation therefore did not change. The replica arithmetic ran anyway, so the same entries
// were counted twice: the annotation described one binding and Spec.Replica described two,
// and nothing downstream brings the two back together. An add counted twice buys a machine
// nobody asked for; a remove counted twice releases one.
func TestWorkspaceMutateNodesActionRefusesASecondRequest(t *testing.T) {
	scheme := newScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		inFlightNode(t, "node-n", ""),       // the pending add
		inFlightNode(t, "node-held", "ws1"), // already ours: an add of it is skipped
		inFlightNode(t, "node-gone", "ws1"), // the pending remove
		inFlightNode(t, "node-free", ""),    // not ours: a remove of it is skipped
	).Build()
	m := &WorkspaceMutator{Client: cli}

	cases := []struct {
		name             string
		inFlight, second string
		replica          int
	}{{
		name:     "a different request",
		inFlight: `{"node-n":"add"}`,
		second:   `{"node-n":"add","node-free":"add"}`,
		replica:  1,
	}, {
		// Shrinks onto the in-flight set: node-held is already bound to ws1, so the add of
		// it is skipped and what is left is {"node-n":"add"} again.
		name:     "an add that shrinks onto the one in flight",
		inFlight: `{"node-n":"add"}`,
		second:   `{"node-n":"add","node-held":"add"}`,
		replica:  1,
	}, {
		// The same, the other way round: node-free is bound to nobody, so the remove of it
		// is skipped.
		name:     "a remove that shrinks onto the one in flight",
		inFlight: `{"node-gone":"remove"}`,
		second:   `{"node-gone":"remove","node-free":"remove"}`,
		replica:  3,
	}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
				Spec: v1.WorkspaceSpec{Cluster: "cluster1", NodeFlavor: "flavor1", Replica: c.replica}}
			v1.SetAnnotation(oldWs, v1.WorkspaceNodesAction, c.inFlight)
			newWs := oldWs.DeepCopy()
			v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, c.second)

			err := m.mutateNodesAction(context.Background(), oldWs, newWs)
			assert.Assert(t, err != nil)
			assert.Assert(t, strings.Contains(err.Error(), "another job"), err.Error())
			// Refused whole. The count is the half that used to leak through.
			assert.Equal(t, newWs.Spec.Replica, c.replica)
		})
	}
}

// Lowering Spec.Replica while a nodes-action is in flight is the same collision arriving as
// two writes instead of one, and is refused the same way.
//
// The admitted case was a new count at or above what the workspace currently holds:
// mutateScaleDown wrote no request of its own and returned, so the lowered count simply
// stood. The pending add then landed, putting the workspace one over, and the next reconcile
// scaled down -- releasing whichever node scale-down picked, which is not the node anybody
// had been talking about.
func TestWorkspaceAdmitRefusesAScaleDownDuringANodesAction(t *testing.T) {
	scheme := newScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		inFlightNode(t, "node-n", ""), inFlightNode(t, "node-held", "ws1"),
		&v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "flavor1"}},
		&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}},
	).Build()
	m, v := &WorkspaceMutator{Client: cli}, &WorkspaceValidator{Client: cli}

	// Holds one node and has an add of a second in flight, so Spec.Replica reads 2.
	for _, target := range []int{1, 0} {
		stored := validWorkspace("ws1")
		stored.Spec.NodeFlavor = "flavor1"
		stored.Spec.Replica = 2
		stored.Status.AvailableReplica = 1
		v1.SetAnnotation(stored, v1.WorkspaceNodesAction, `{"node-n":"add"}`)
		incoming := stored.DeepCopy()
		incoming.Spec.Replica = target

		err := admit(t, m, v, stored, incoming)
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "another job"), err.Error())
		// Untouched: no scale-down request written over the one in flight.
		assert.Equal(t, v1.GetWorkspaceNodesAction(incoming), `{"node-n":"add"}`)
	}

	// Raising it is not this. An add in flight and a higher target are the same intent
	// arriving twice, and the count the request moved is not spent twice by allowing it.
	stored := validWorkspace("ws1")
	stored.Spec.NodeFlavor = "flavor1"
	stored.Spec.Replica = 2
	stored.Status.AvailableReplica = 1
	v1.SetAnnotation(stored, v1.WorkspaceNodesAction, `{"node-n":"add"}`)
	incoming := stored.DeepCopy()
	incoming.Spec.Replica = 5
	assert.NilError(t, admit(t, m, v, stored, incoming))
}

// TestWorkspaceAdmitLetsTheControllerEndANodesAction covers the two writes that finish a
// request, both of which change the annotation while one is in flight and so run straight
// into the check that refuses a second request.
//
// The clear is the one that matters most: it is how every successful request ends, it is an
// ordinary patch with no shape to recognise it by, and refusing it strands the request in
// flight forever -- which under the same in-flight rule freezes every later nodes-action and
// scale-down on the workspace.
func TestWorkspaceAdmitLetsTheControllerEndANodesAction(t *testing.T) {
	scheme := newScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		inFlightNode(t, "node-n", "ws1"), inFlightNode(t, "node-x", ""),
		&v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "flavor1"}},
		&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}},
	).Build()
	m, v := &WorkspaceMutator{Client: cli}, &WorkspaceValidator{Client: cli}

	stored := func(action string) *v1.Workspace {
		w := validWorkspace("ws1")
		w.Spec.NodeFlavor = "flavor1"
		w.Spec.Replica = 2
		w.Status.AvailableReplica = 1
		v1.SetAnnotation(w, v1.WorkspaceNodesAction, action)
		return w
	}

	// removeNodesAction: the request is done, so the annotation goes. Nothing else moves.
	old := stored(`{"node-n":"add"}`)
	incoming := old.DeepCopy()
	v1.RemoveAnnotation(incoming, v1.WorkspaceNodesAction)
	assert.NilError(t, admit(t, m, v, old, incoming))
	assert.Equal(t, v1.GetWorkspaceNodesAction(incoming), "")
	assert.Equal(t, incoming.Spec.Replica, 2)

	// dropRefusedActions withdrawing the only entry: the annotation goes the same way, but
	// with a reason and the replica the entry was counted into given back.
	old = stored(`{"node-x":"add"}`)
	incoming = old.DeepCopy()
	v1.RemoveAnnotation(incoming, v1.WorkspaceNodesAction)
	v1.SetAnnotation(incoming, v1.WorkspaceNodesActionError, "node-x: it no longer exists")
	incoming.Spec.Replica = 1
	assert.NilError(t, admit(t, m, v, old, incoming))
	assert.Equal(t, incoming.Spec.Replica, 1)
}
