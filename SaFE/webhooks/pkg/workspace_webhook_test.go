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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

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
				v1.WorkloadStickyNodesAnnotation: v1.TrueStr,
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
	assert.Equal(t, v1.GetAnnotation(updated, v1.WorkloadStickyNodesAnnotation), "")
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
