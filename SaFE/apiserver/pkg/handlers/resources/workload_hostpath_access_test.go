/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const (
	hpWorkspace = "ws-hostpath"
	hpOther     = "ws-other"
	// hpMember is a member of hpWorkspace.
	hpMember = "hp-member"
	// hpManager manages hpWorkspace.
	hpManager = "hp-manager"
	// hpOtherManager is a member of hpWorkspace and manages another workspace.
	hpOtherManager = "hp-other-manager"
	hpAdmin        = "hp-admin"
)

type hostPathFixture struct {
	h   *Handler
	cli client.Client
}

// newHostPathFixture is a cluster with the built-in roles as the chart ships them, a workspace that
// declares /shared_nfs as a hostpath volume for every scope, /apps for training only, a read-only
// volume that shows only /teams/a, a volume with user directories over /home and a PFS volume, the
// control-plane node the create path reads, and users in different relations to the workspace.
func newHostPathFixture(t *testing.T) *hostPathFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	user := func(name string, workspaces, managed []string, roles ...v1.UserRole) *v1.User {
		u := &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name},
				Annotations: map[string]string{v1.UserNameAnnotation: name}},
			Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Resources: map[string][]string{}},
		}
		if len(workspaces) > 0 {
			u.Spec.Resources[common.UserWorkspaces] = workspaces
		}
		if len(managed) > 0 {
			u.Spec.Resources[common.UserManagedWorkspaces] = managed
		}
		return u
	}
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: hpWorkspace},
		Spec: v1.WorkspaceSpec{Cluster: "c1", Volumes: []v1.WorkspaceVolume{
			{Id: 1, Type: v1.HOSTPATH, HostPath: "/shared_nfs", MountPath: "/shared_nfs"},
			{Id: 2, Type: v1.HOSTPATH, HostPath: "/apps", MountPath: "/apps",
				Scopes: []v1.WorkspaceScope{v1.TrainScope}},
			{Id: 3, Type: v1.PFS, MountPath: "/mnt/ceph", StorageClass: "sc", Capacity: "1Ti"},
			{Id: 4, Type: v1.HOSTPATH, HostPath: "/teams", SubPath: "a", MountPath: "/team",
				AccessMode: corev1.ReadOnlyMany},
			{Id: 5, Type: v1.HOSTPATH, HostPath: "/home", MountPath: "/home", EnableUserDir: true},
		}},
	}
	controlPlane := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "cp-1", Labels: map[string]string{common.KubernetesControlPlane: ""}},
		Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{
			{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
		}},
	}
	ws := []string{hpWorkspace}
	objs := append(authtest.ChartRoles(t), workspace, controlPlane,
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: hpOther}},
		user(hpMember, ws, nil, v1.DefaultRole),
		user(hpManager, ws, ws, v1.DefaultRole),
		user(hpOtherManager, []string{hpWorkspace, hpOther}, []string{hpOther}, v1.DefaultRole),
		user(hpAdmin, nil, nil, v1.SystemAdminRole),
	)
	cli := fake.NewClientBuilder().WithScheme(getTestScheme()).WithObjects(objs...).
		WithStatusSubresource(&v1.Workload{}).Build()
	return &hostPathFixture{
		h:   &Handler{Client: cli, accessController: &authority.AccessController{Client: cli}},
		cli: cli,
	}
}

// create calls the create handler as the REST route does, with a workload of the given kind that
// asks for hostpath, and returns the stored workload when it was created.
func (f *hostPathFixture) create(t *testing.T, userId, kind string, hostpath []string,
	annotations map[string]string) (*v1.Workload, error) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"displayName": "hp",
		"workspaceId": hpWorkspace,
		"images":      []string{"img"},
		"entryPoints": []string{"sleep"},
		"groupVersionKind": map[string]string{
			"version": "v1", "kind": kind,
		},
		"resources":   []map[string]any{{"replica": 1, "cpu": "1", "memory": "1Gi"}},
		"hostpath":    hostpath,
		"annotations": annotations,
	})
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, userId)
	res, err := f.h.createWorkload(c)
	if err != nil {
		return nil, err
	}
	stored := &v1.Workload{}
	require.NoError(t, f.cli.Get(t.Context(),
		client.ObjectKey{Name: res.(*view.CreateWorkloadResponse).WorkloadId}, stored))
	return stored, nil
}

func (f *hostPathFixture) workloadCount(t *testing.T) int {
	t.Helper()
	list := &v1.WorkloadList{}
	require.NoError(t, f.cli.List(t.Context(), list))
	return len(list.Items)
}

// A member of the workspace, or a manager of another workspace, cannot create a workload that
// mounts a node path the workspace does not declare for the workload's kind: the request is
// refused with 403 naming the grant and nothing is created. A primus-safe annotation in the request
// cannot stand in for the grant.
func TestMembersCannotMountUndeclaredHostPaths(t *testing.T) {
	for _, tt := range []struct {
		name        string
		user        string
		kind        string
		hostpath    []string
		annotations map[string]string
	}{
		{"node root", hpMember, common.PytorchJobKind, []string{"/"}, nil},
		{"docker socket", hpMember, common.PytorchJobKind, []string{"/var/run/docker.sock"}, nil},
		{"next to a declared path", hpMember, common.PytorchJobKind, []string{"/shared_nfs/a", "/etc"}, nil},
		{"dot-dot out of a declared path", hpMember, common.PytorchJobKind, []string{"/shared_nfs/../etc"}, nil},
		{"a training path in an inference workload", hpMember, common.DeploymentKind, []string{"/apps"}, nil},
		{"a PFS mount path", hpMember, common.AuthoringKind, []string{"/mnt/ceph"}, nil},
		{"above a volume's subPath", hpMember, common.PytorchJobKind, []string{"/teams"}, nil},
		{"next to a volume's subPath", hpMember, common.PytorchJobKind, []string{"/teams/b"}, nil},
		{"a volume with user directories", hpMember, common.PytorchJobKind, []string{"/home"}, nil},
		{"another user's directory", hpMember, common.PytorchJobKind, []string{"/home/users/" + hpManager}, nil},
		{"the user's own directory", hpMember, common.PytorchJobKind, []string{"/home/users/" + hpMember}, nil},
		{"with the grant annotation in the request", hpMember, common.PytorchJobKind, []string{"/etc"},
			map[string]string{v1.WorkloadHostPathAuthzAnnotation: v1.TrueStr}},
		{"as a manager of another workspace", hpOtherManager, common.PytorchJobKind, []string{"/"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newHostPathFixture(t)
			_, err := f.create(t, tt.user, tt.kind, tt.hostpath, tt.annotations)
			require.True(t, apierrors.IsForbidden(err), "want 403, got %v", err)
			require.Contains(t, err.Error(), "requires the workload/privileged grant")
			require.Equal(t, 0, f.workloadCount(t), "a refused workload was created")
		})
	}
}

// A member may still name a hostpath volume the workspace declares for the workload's kind, or a
// directory under one; the workload carries no grant mark, so the webhook judges it as declared.
func TestMembersMountDeclaredHostPaths(t *testing.T) {
	f := newHostPathFixture(t)
	stored, err := f.create(t, hpMember, common.PytorchJobKind,
		[]string{"/shared_nfs/team/a", "/apps", "/apps/data/"}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"/shared_nfs/team/a", "/apps", "/apps/data/"}, stored.Spec.Hostpath)
	require.False(t, v1.HasAnnotation(stored, v1.WorkloadHostPathAuthzAnnotation))

	stored, err = f.create(t, hpMember, common.DeploymentKind, []string{"/shared_nfs"}, nil)
	require.NoError(t, err)
	require.False(t, v1.HasAnnotation(stored, v1.WorkloadHostPathAuthzAnnotation))

	// What a volume with a subPath declares: hostPath/subPath and directories under it; and what a
	// volume with user directories declares: the directories under it but its user directories.
	stored, err = f.create(t, hpMember, common.PytorchJobKind, []string{"/teams/a", "/teams/a/x", "/home/models"}, nil)
	require.NoError(t, err)
	require.False(t, v1.HasAnnotation(stored, v1.WorkloadHostPathAuthzAnnotation))
}

// The workspace's manager and a system administrator hold workload/privileged in the workspace, so
// they may mount any node path; the workload is marked so the webhook admits it.
func TestPrivilegedGrantMountsUndeclaredHostPaths(t *testing.T) {
	for _, who := range []string{hpManager, hpAdmin} {
		t.Run(who, func(t *testing.T) {
			f := newHostPathFixture(t)
			stored, err := f.create(t, who, common.PytorchJobKind, []string{"/", "/shared_nfs/a"}, nil)
			require.NoError(t, err)
			require.Equal(t, v1.TrueStr, v1.GetAnnotation(stored, v1.WorkloadHostPathAuthzAnnotation))

			// Without an undeclared entry there is nothing to grant and no mark.
			stored, err = f.create(t, who, common.PytorchJobKind, []string{"/shared_nfs/a"}, nil)
			require.NoError(t, err)
			require.False(t, v1.HasAnnotation(stored, v1.WorkloadHostPathAuthzAnnotation))
		})
	}
}

// The grant mark is set only by the check: a workload that reaches the create path already
// carrying it (resume and preheat build the workload themselves) loses it unless the check sets it.
func TestHostPathGrantMarkIsSetOnlyByTheCheck(t *testing.T) {
	f := newHostPathFixture(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)
	c.Set(common.UserId, hpMember)
	member := &v1.User{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: hpMember}, member))

	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "preset", Annotations: map[string]string{
			v1.WorkloadHostPathAuthzAnnotation: v1.TrueStr,
		}},
		Spec: v1.WorkloadSpec{
			Workspace:        hpWorkspace,
			GroupVersionKind: v1.GroupVersionKind{Version: "v1", Kind: common.PytorchJobKind},
			Hostpath:         []string{"/shared_nfs/a"},
		},
	}
	_, err := f.h.createWorkloadImpl(c, w.DeepCopy(), member, nil)
	require.NoError(t, err)
	stored := &v1.Workload{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: "preset"}, stored))
	require.False(t, v1.HasAnnotation(stored, v1.WorkloadHostPathAuthzAnnotation))

	w.Name = "preset-undeclared"
	w.Spec.Hostpath = []string{"/etc"}
	_, err = f.h.createWorkloadImpl(c, w.DeepCopy(), member, nil)
	require.True(t, apierrors.IsForbidden(err), "want 403, got %v", err)
	require.True(t, apierrors.IsNotFound(f.cli.Get(t.Context(), client.ObjectKey{Name: "preset-undeclared"}, &v1.Workload{})))
}
