/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	"gotest.tools/v3/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// optOutExternalJob returns an external PyTorchJob of the user alice with the given annotations.
func optOutExternalJob(annotations map[string]string) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "opt-out",
			Labels:      map[string]string{v1.UserIdLabel: "alice"},
			Annotations: annotations,
		},
		Spec:   v1.WorkloadSpec{GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind}},
		Status: v1.WorkloadStatus{ExternalExecution: &v1.WorkloadExternalExecution{ClaimId: "claim-1"}},
	}
}

// With external_execution.user_dir_mounts on, an external workload may name a path below the
// declared root; an opted-out workload is held to the root alone, as when the setting is off.
func TestUserDirMountsOffWorkloadRestoresRootOnlyHostPath(t *testing.T) {
	withUserDirMounts(t, true)
	ws := sandboxRootWorkspace()

	assert.NilError(t, checkExternalHostPath(optOutExternalJob(nil), ws, "/data/train/x"))

	off := optOutExternalJob(map[string]string{v1.WorkloadUserDirMountsOffAnnotation: v1.TrueStr})
	assert.ErrorContains(t, checkExternalHostPath(off, ws, "/data/train/x"),
		`hostpath "/data/train/x" of an external workload is below the declared root "/data/train"`)
	assert.NilError(t, checkExternalHostPath(off, ws, "/data/train"))
}

// An opted-out workload gets no user directory of a volume that enables them: the whole volume is
// mounted, as when the setting is off.
func TestUserDirMountsOffWorkloadMountsWholeVolume(t *testing.T) {
	withUserDirMounts(t, true)
	vol := v1.WorkspaceVolume{
		Id: 1, Type: v1.HOSTPATH, HostPath: "/shared_nfs", MountPath: "/shared_nfs", EnableUserDir: true,
	}

	mounts, handled, err := externalUserDirMounts(optOutExternalJob(nil), vol)
	assert.NilError(t, err)
	assert.Assert(t, handled)
	assert.Assert(t, len(mounts) > 0)

	off := optOutExternalJob(map[string]string{v1.WorkloadUserDirMountsOffAnnotation: v1.TrueStr})
	mounts, handled, err = externalUserDirMounts(off, vol)
	assert.NilError(t, err)
	assert.Assert(t, !handled)
	assert.Equal(t, len(mounts), 0)
}

// The Pod-level user directory check follows the same switch: a Pod that overlaps the user directories
// is refused for a workload that keeps the setting, and admitted for an opted-out one.
func TestUserDirMountsOffWorkloadSkipsPodUserDirCheck(t *testing.T) {
	withUserDirMounts(t, true)
	spec := jobutils.TestSandboxResourceTemplate.Spec.ResourceSpecs[0]
	ws := topModelsWorkspace()
	newWorkload := func(annotations map[string]string) *v1.Workload {
		w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{v1.UserIdLabel: "user-1"}, Annotations: annotations,
		}}
		externalPlacement(w)
		w.Spec.GroupVersionKind = v1.GroupVersionKind{Kind: common.SandboxKind, Version: common.DefaultVersion}
		return w
	}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	volumes := []interface{}{buildHostPathVolume("tmpl-0", "/shared_nfs/users/user-10", "Directory")}
	assert.NilError(t, jobutils.SetNestedField(obj.Object, volumes, podSpecPath(newWorkload(nil), &spec, "volumes")))

	assert.ErrorContains(t, checkExternalPodHostPaths(obj, newWorkload(nil), ws, spec), "overlaps the user directories")
	off := newWorkload(map[string]string{v1.WorkloadUserDirMountsOffAnnotation: v1.TrueStr})
	assert.NilError(t, checkExternalPodHostPaths(obj, off, ws, spec))
}
