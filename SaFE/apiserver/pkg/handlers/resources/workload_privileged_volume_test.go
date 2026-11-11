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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// newPrivilegedVolumeFixture is newHostPathFixture with the PFS volume (/mnt/ceph) and a hostpath
// volume over /secret that only the holders of workload/privileged get (requiresPrivilegedGrant).
func newPrivilegedVolumeFixture(t *testing.T) *hostPathFixture {
	t.Helper()
	f := newHostPathFixture(t)
	ws := &v1.Workspace{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: hpWorkspace}, ws))
	for i := range ws.Spec.Volumes {
		if ws.Spec.Volumes[i].Type == v1.PFS {
			ws.Spec.Volumes[i].RequiresPrivilegedGrant = true
		}
	}
	ws.Spec.Volumes = append(ws.Spec.Volumes, v1.WorkspaceVolume{
		Id: 6, Type: v1.HOSTPATH, HostPath: "/secret", MountPath: "/secret", RequiresPrivilegedGrant: true,
	})
	require.NoError(t, f.cli.Update(t.Context(), ws))
	return f
}

// A member's workload is created without the mark, so the dispatcher leaves the volumes that
// require the grant out of it; a primus-safe annotation in the request cannot set the mark.
func TestMembersDoNotGetPrivilegedVolumes(t *testing.T) {
	for _, annotations := range []map[string]string{
		nil,
		{v1.WorkloadPrivilegedVolumesAuthzAnnotation: v1.TrueStr},
		{v1.WorkloadPrivilegedVolumesAuthzAnnotation: v1.TrueStr, v1.WorkloadHostPathAuthzAnnotation: v1.TrueStr},
	} {
		for _, who := range []string{hpMember, hpOtherManager} {
			f := newPrivilegedVolumeFixture(t)
			stored, err := f.create(t, who, common.PytorchJobKind, nil, annotations)
			require.NoError(t, err, "%s %v", who, annotations)
			require.False(t, v1.HasAnnotation(stored, v1.WorkloadPrivilegedVolumesAuthzAnnotation),
				"%s got the mark with request annotations %v", who, annotations)
			require.False(t, v1.HasAnnotation(stored, v1.WorkloadHostPathAuthzAnnotation))
		}
	}
}

// A member who asks for a volume that requires the grant, by naming its hostPath, a path under it
// or a path containing it in spec.hostpath, is refused with 403 naming the volume and the grant,
// and nothing is created; a forged mark does not help.
func TestMembersAskingForPrivilegedVolumesAreRefused(t *testing.T) {
	for _, tt := range []struct {
		name        string
		hostpath    []string
		annotations map[string]string
	}{
		{"its hostPath", []string{"/secret"}, nil},
		{"a path under it", []string{"/shared_nfs/a", "/secret/keys"}, nil},
		{"a path containing it", []string{"/"}, nil},
		{"with forged marks", []string{"/secret"}, map[string]string{
			v1.WorkloadPrivilegedVolumesAuthzAnnotation: v1.TrueStr, v1.WorkloadHostPathAuthzAnnotation: v1.TrueStr}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newPrivilegedVolumeFixture(t)
			_, err := f.create(t, hpMember, common.PytorchJobKind, tt.hostpath, tt.annotations)
			require.True(t, apierrors.IsForbidden(err), "want 403, got %v", err)
			require.Contains(t, err.Error(), "requiresPrivilegedGrant")
			require.Contains(t, err.Error(), "volume 6")
			require.Contains(t, err.Error(), "requires the workload/privileged grant")
			require.Equal(t, 0, f.workloadCount(t), "a refused workload was created")
		})
	}
}

// The workspace's manager and a system administrator hold workload/privileged in the workspace:
// their workloads are marked, so the dispatcher mounts the volumes that require the grant, and they
// may name such a volume's hostPath.
func TestPrivilegedGrantGetsPrivilegedVolumes(t *testing.T) {
	for _, who := range []string{hpManager, hpAdmin} {
		t.Run(who, func(t *testing.T) {
			f := newPrivilegedVolumeFixture(t)
			stored, err := f.create(t, who, common.PytorchJobKind, nil, nil)
			require.NoError(t, err)
			require.Equal(t, v1.TrueStr, v1.GetAnnotation(stored, v1.WorkloadPrivilegedVolumesAuthzAnnotation))

			stored, err = f.create(t, who, common.PytorchJobKind, []string{"/secret/keys"}, nil)
			require.NoError(t, err)
			require.Equal(t, v1.TrueStr, v1.GetAnnotation(stored, v1.WorkloadPrivilegedVolumesAuthzAnnotation))
			require.Equal(t, v1.TrueStr, v1.GetAnnotation(stored, v1.WorkloadHostPathAuthzAnnotation))
		})
	}
}

// Where no volume requires the grant there is
// nothing to grant and no mark, whoever creates the workload.
func TestPrivilegedVolumeMarkOnlyWhenItDecides(t *testing.T) {
	f := newHostPathFixture(t)
	stored, err := f.create(t, hpAdmin, common.PytorchJobKind, nil, nil)
	require.NoError(t, err)
	require.False(t, v1.HasAnnotation(stored, v1.WorkloadPrivilegedVolumesAuthzAnnotation))
}

// The mark is set only by the check: a workload that reaches the create path already carrying it
// (resume and preheat build the workload themselves) loses it when its creator lacks the grant.
func TestPrivilegedVolumeMarkIsSetOnlyByTheCheck(t *testing.T) {
	f := newPrivilegedVolumeFixture(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)
	c.Set(common.UserId, hpMember)
	member := &v1.User{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: hpMember}, member))
	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "preset", Annotations: map[string]string{
			v1.WorkloadPrivilegedVolumesAuthzAnnotation: v1.TrueStr,
		}},
		Spec: v1.WorkloadSpec{
			Workspace:        hpWorkspace,
			GroupVersionKind: v1.GroupVersionKind{Version: "v1", Kind: common.PytorchJobKind},
		},
	}
	_, err := f.h.createWorkloadImpl(c, w, member, nil)
	require.NoError(t, err)
	stored := &v1.Workload{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: "preset"}, stored))
	require.False(t, v1.HasAnnotation(stored, v1.WorkloadPrivilegedVolumesAuthzAnnotation))
}

// A system administrator who creates a workload on behalf of another user (userEntity, as the
// runner proxy does for CI jobs) gives it the volumes that require the grant only when that user
// holds the grant as well, since the workload runs as them.
func TestPrivilegedVolumesOnBehalfOfAnotherUser(t *testing.T) {
	for _, tt := range []struct {
		owner string
		want  bool
	}{
		{hpMember, false},
		{hpOtherManager, false},
		{"no-such-user", false},
		{hpManager, true},
	} {
		t.Run(tt.owner, func(t *testing.T) {
			f := newPrivilegedVolumeFixture(t)
			body, err := json.Marshal(map[string]any{
				"displayName": "hp",
				"workspaceId": hpWorkspace,
				"images":      []string{"img"},
				"entryPoints": []string{"sleep"},
				"groupVersionKind": map[string]string{
					"version": "v1", "kind": common.PytorchJobKind,
				},
				"resources":  []map[string]any{{"replica": 1, "cpu": "1", "memory": "1Gi"}},
				"userEntity": map[string]string{"id": tt.owner, "name": tt.owner},
			})
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", strings.NewReader(string(body)))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(common.UserId, hpAdmin)
			res, err := f.h.createWorkload(c)
			require.NoError(t, err)
			stored := &v1.Workload{}
			require.NoError(t, f.cli.Get(t.Context(),
				client.ObjectKey{Name: res.(*view.CreateWorkloadResponse).WorkloadId}, stored))
			require.Equal(t, tt.owner, v1.GetUserId(stored))
			require.Equal(t, tt.want, v1.HasAnnotation(stored, v1.WorkloadPrivilegedVolumesAuthzAnnotation))
		})
	}
}

// A workload whose workspace does not exist is refused when it names hostpath entries, which the
// workspace must declare, and created without a mark when it names none.
func TestPrivilegedVolumeCheckWithoutWorkspace(t *testing.T) {
	f := newPrivilegedVolumeFixture(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)
	c.Set(common.UserId, hpAdmin)
	admin := &v1.User{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: hpAdmin}, admin))
	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "no-ws", Annotations: map[string]string{
			v1.WorkloadPrivilegedVolumesAuthzAnnotation: v1.TrueStr}},
		Spec: v1.WorkloadSpec{
			Workspace:        "nope",
			GroupVersionKind: v1.GroupVersionKind{Version: "v1", Kind: common.PytorchJobKind},
			Hostpath:         []string{"/shared_nfs"},
		},
	}
	_, err := f.h.createWorkloadImpl(c, w.DeepCopy(), admin, nil)
	require.True(t, apierrors.IsNotFound(err), "want not found, got %v", err)

	w.Spec.Hostpath = nil
	_, err = f.h.createWorkloadImpl(c, w.DeepCopy(), admin, nil)
	require.NoError(t, err)
	stored := &v1.Workload{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: "no-ws"}, stored))
	require.False(t, v1.HasAnnotation(stored, v1.WorkloadPrivilegedVolumesAuthzAnnotation))
}
