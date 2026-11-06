/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// privilegedVolumeWorkspace is scopedWorkspace(everyScopeButSandbox) with pfs-1 requiring the
// workload/privileged grant, as global-cpu's shared volume is meant to.
func privilegedVolumeWorkspace() *v1.Workspace {
	ws := scopedWorkspace(everyScopeButSandbox)
	ws.Spec.Volumes[0].RequiresPrivilegedGrant = true
	return ws
}

// A volume with requiresPrivilegedGrant is rendered, through the dispatcher's create path, only for
// a workload the apiserver marked as created with the grant: an unmarked workload of a listed scope
// gets neither the volume, its mount nor USER_DATA_PATH, while the other volume and its own
// hostpath keep their names.
func TestPrivilegedVolumeRenderedOnlyForMarkedWorkloads(t *testing.T) {
	for _, kind := range []string{common.PytorchJobKind, common.JobKind, common.DeploymentKind} {
		t.Run(kind, func(t *testing.T) {
			obj, workload, spec := renderWorkload(t, kind, privilegedVolumeWorkspace(), []string{"/opt/extra"}, nil)
			volumes, mounts, userDataPath := summarizeVolumes(t, obj, workload, spec)
			assert.Assert(t, !volumes["pfs-1"], "pfs-1 rendered for an unmarked workload: %v", volumes)
			_, mounted := mounts["pfs-1"]
			assert.Assert(t, !mounted, "pfs-1 mounted into an unmarked workload: %v", mounts)
			assert.Assert(t, !userDataPath, "USER_DATA_PATH from a volume the workload does not get")
			assert.Assert(t, volumes["hostpath-2"])
			assert.Assert(t, volumes["hostpath-3"])

			obj, workload, spec = renderWorkload(t, kind, privilegedVolumeWorkspace(), []string{"/opt/extra"},
				func(w *v1.Workload) {
					metav1.SetMetaDataAnnotation(&w.ObjectMeta, v1.WorkloadPrivilegedVolumesAuthzAnnotation, v1.TrueStr)
				})
			volumes, mounts, userDataPath = summarizeVolumes(t, obj, workload, spec)
			assert.Assert(t, volumes["pfs-1"], "pfs-1 missing for a marked workload: %v", volumes)
			assert.Equal(t, mounts["pfs-1"], "/ceph/users/user-1")
			assert.Assert(t, userDataPath)
			assert.Assert(t, volumes["hostpath-3"])
		})
	}
}

// The mark does not widen the volume's scopes: a marked Sandbox still does not get a volume whose
// scopes leave Sandbox out. Any value but "true" is no mark.
func TestPrivilegedVolumeMarkKeepsScopes(t *testing.T) {
	for _, tt := range []struct {
		kind, mark string
		want       bool
	}{
		{common.SandboxKind, v1.TrueStr, false},
		{common.AuthoringKind, "yes", false},
		{common.AuthoringKind, "", false},
		{common.AuthoringKind, v1.TrueStr, true},
	} {
		workload := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
			Name:        "w",
			Labels:      map[string]string{v1.UserIdLabel: "user-1"},
			Annotations: map[string]string{v1.UseWorkspaceStorageAnnotation: v1.TrueStr},
		}}
		if tt.mark != "" {
			workload.Annotations[v1.WorkloadPrivilegedVolumesAuthzAnnotation] = tt.mark
		}
		workload.Spec.GroupVersionKind.Kind = tt.kind
		ws := privilegedVolumeWorkspace()
		obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
		assert.NilError(t, modifyVolumes(obj, workload, ws, []string{"volumes"}))
		hasVolume := false
		vols, _, _ := jobutils.NestedSlice(obj.Object, []string{"volumes"})
		for _, v := range vols {
			if v.(map[string]interface{})["name"] == "pfs-1" {
				hasVolume = true
			}
		}
		assert.Equal(t, hasVolume, tt.want, "kind %s mark %q", tt.kind, tt.mark)
		wantNfs := "/data"
		if tt.want {
			wantNfs = "/ceph"
		}
		assert.Equal(t, getNfsPathFromWorkspace(workload, ws), wantNfs, "kind %s mark %q", tt.kind, tt.mark)
	}
}

// A hostpath entry that reaches a volume requiring the grant is not mounted into a workload whose
// creator was not found to hold it (a workload created before the volume required the grant
// keeps such an entry): dispatching fails with the reason. The apiserver's hostpath grant mark, or
// a privileged workload, mounts it.
func TestPrivilegedVolumeHostPathNotRenderedWithoutTheGrant(t *testing.T) {
	ws := func() *v1.Workspace {
		ws := scopedWorkspace(nil)
		ws.Spec.Volumes[1].RequiresPrivilegedGrant = true // hostpath-2 over /apps
		return ws
	}
	for _, hostpath := range []string{"/apps", "/apps/keys", "/"} {
		_, _, _, err := renderWorkloadErr(t, common.PytorchJobKind, ws(), []string{hostpath}, nil)
		assert.ErrorContains(t, err, "requiresPrivilegedGrant")
		assert.ErrorContains(t, err, `"`+hostpath+`"`)
	}
	_, _, _, err := renderWorkloadErr(t, common.PytorchJobKind, ws(), []string{"/opt/extra"}, nil)
	assert.NilError(t, err)

	for _, mark := range []string{v1.WorkloadHostPathAuthzAnnotation, v1.WorkloadPrivilegedAnnotation} {
		_, _, _, err = renderWorkloadErr(t, common.PytorchJobKind, ws(), []string{"/apps/keys"},
			func(w *v1.Workload) { metav1.SetMetaDataAnnotation(&w.ObjectMeta, mark, v1.TrueStr) })
		assert.NilError(t, err, mark)
	}
	// The privileged-volume mark alone is not the hostpath grant.
	_, _, _, err = renderWorkloadErr(t, common.PytorchJobKind, ws(), []string{"/apps/keys"},
		func(w *v1.Workload) {
			metav1.SetMetaDataAnnotation(&w.ObjectMeta, v1.WorkloadPrivilegedVolumesAuthzAnnotation, v1.TrueStr)
		})
	assert.ErrorContains(t, err, "requiresPrivilegedGrant")
}
