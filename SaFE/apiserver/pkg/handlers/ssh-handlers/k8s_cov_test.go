/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestIsSlurmLoginPod(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name: "slurm login pod",
			labels: map[string]string{
				"app.kubernetes.io/part-of":   "slurm",
				"app.kubernetes.io/component": "login",
			},
			expected: true,
		},
		{
			name: "slurm worker pod",
			labels: map[string]string{
				"app.kubernetes.io/part-of":   "slurm",
				"app.kubernetes.io/component": "worker",
			},
			expected: false,
		},
		{
			name:     "no labels",
			labels:   nil,
			expected: false,
		},
		{
			name: "login component but not slurm",
			labels: map[string]string{
				"app.kubernetes.io/part-of":   "other",
				"app.kubernetes.io/component": "login",
			},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: tt.labels}}
			assert.Equal(t, tt.expected, isSlurmLoginPod(pod))
		})
	}
}

// authUserTestHandler builds an SshHandler whose access controller is backed by
// a fake client seeded with an admin user/role and the given workspaces, so the
// nil-workload (Slurm) branch of authUser can be exercised end to end.
func authUserTestHandler(objs ...client.Object) *SshHandler {
	adminUser := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "admin",
			Labels:      map[string]string{v1.UserIdLabel: "admin"},
			Annotations: map[string]string{v1.UserNameAnnotation: "admin"},
		},
		Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
	}
	adminRole := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.SystemAdminRole)},
		Rules: []v1.PolicyRule{{
			Resources:    []string{authority.AllResource},
			Verbs:        []v1.RoleVerb{v1.AllVerb},
			GrantedUsers: []string{authority.GrantedAllUser},
		}},
	}
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	all := append([]client.Object{adminUser, adminRole}, objs...)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(all...).Build()
	return &SshHandler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}
}

func TestAuthUserSlurmLoginPod(t *testing.T) {
	slurmWs := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-slurm"},
		Spec:       v1.WorkspaceSpec{Scopes: []v1.WorkspaceScope{v1.SlurmScope}},
	}
	noScopeWs := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-noscope"},
		Spec:       v1.WorkspaceSpec{},
	}
	h := authUserTestHandler(slurmWs, noScopeWs)

	// Workspace with Slurm scope + admin user: authorized.
	err := h.authUser(context.Background(), &UserInfo{User: "admin", Namespace: "ws-slurm"}, nil)
	assert.NoError(t, err)

	// Workspace without Slurm scope: rejected.
	err = h.authUser(context.Background(), &UserInfo{User: "admin", Namespace: "ws-noscope"}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Slurm scope")

	// Missing workspace object: workspace lookup fails.
	err = h.authUser(context.Background(), &UserInfo{User: "admin", Namespace: "ws-absent"}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace")
}

// getClientsTestHandler builds an SshHandler with a control-plane fake client
// (holding the given workspace/workload objects) and a data-plane clientset
// (holding the given pods) wired through an ObjectManager keyed by cluster.
func getClientsTestHandler(ctrlObjs []client.Object, pods ...runtime.Object) *SshHandler {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	ctrlFake := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ctrlObjs...).Build()

	cs := k8sfake.NewSimpleClientset(pods...)
	cf := commonclient.NewClientFactoryWithOnlyClient(context.Background(), "cluster", cs)
	om := commonutils.NewObjectManager()
	om.AddOrReplace("cluster", cf)
	return &SshHandler{Client: ctrlFake, clientManager: om}
}

func TestGetWorkloadAndClients(t *testing.T) {
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
		Spec:       v1.WorkspaceSpec{Cluster: "cluster"},
	}
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-1"},
		Spec:       v1.WorkloadSpec{Workspace: "ws-1"},
	}
	loginPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "login-0",
		Namespace: "ws-1",
		Labels: map[string]string{
			"app.kubernetes.io/part-of":   "slurm",
			"app.kubernetes.io/component": "login",
		},
	}}
	workloadPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "wl-pod",
		Namespace: "ws-1",
		Labels:    map[string]string{v1.WorkloadIdLabel: "wl-1"},
	}}
	plainPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "plain-pod",
		Namespace: "ws-1",
	}}

	t.Run("slurm login pod returns nil workload with clients", func(t *testing.T) {
		h := getClientsTestHandler([]client.Object{workspace}, loginPod)
		wl, clients, err := h.getWorkloadAndClients(context.Background(),
			&UserInfo{Namespace: "ws-1", Pod: "login-0"})
		assert.NoError(t, err)
		assert.Nil(t, wl)
		assert.NotNil(t, clients)
	})

	t.Run("workload pod resolves the workload", func(t *testing.T) {
		h := getClientsTestHandler([]client.Object{workspace, workload}, workloadPod)
		wl, clients, err := h.getWorkloadAndClients(context.Background(),
			&UserInfo{Namespace: "ws-1", Pod: "wl-pod"})
		assert.NoError(t, err)
		assert.NotNil(t, clients)
		if assert.NotNil(t, wl) {
			assert.Equal(t, "wl-1", wl.Name)
		}
	})

	t.Run("pod not found", func(t *testing.T) {
		h := getClientsTestHandler([]client.Object{workspace})
		_, _, err := h.getWorkloadAndClients(context.Background(),
			&UserInfo{Namespace: "ws-1", Pod: "missing"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get pod")
	})

	t.Run("non-slurm pod without workload id", func(t *testing.T) {
		h := getClientsTestHandler([]client.Object{workspace}, plainPod)
		_, _, err := h.getWorkloadAndClients(context.Background(),
			&UserInfo{Namespace: "ws-1", Pod: "plain-pod"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workload id")
	})

	t.Run("missing workspace", func(t *testing.T) {
		h := getClientsTestHandler(nil, loginPod)
		_, _, err := h.getWorkloadAndClients(context.Background(),
			&UserInfo{Namespace: "ws-1", Pod: "login-0"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get namespace")
	})
}
