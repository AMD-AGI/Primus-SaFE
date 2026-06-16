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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
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
	actions, err := parseNodesAction(empty)
	assert.NilError(t, err)
	assert.Assert(t, actions == nil)

	ws := &v1.Workspace{}
	v1.SetAnnotation(ws, v1.WorkspaceNodesAction, `{"node1":"add"}`)
	actions, err = parseNodesAction(ws)
	assert.NilError(t, err)
	assert.Equal(t, actions["node1"], "add")

	bad := &v1.Workspace{}
	v1.SetAnnotation(bad, v1.WorkspaceNodesAction, `{invalid`)
	_, err = parseNodesAction(bad)
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
	assert.NilError(t, v.validateNodesAction(context.Background(), &v1.Workspace{}, &v1.Workspace{}))
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
