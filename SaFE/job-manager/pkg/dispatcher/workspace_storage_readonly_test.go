/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"strings"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// readOnlyStorage sets read-only workspace storage on the workload.
func readOnlyStorage(w *v1.Workload) {
	metav1.SetMetaDataAnnotation(&w.ObjectMeta, v1.WorkspaceStorageReadOnlyAnnotation, v1.TrueStr)
}

// externalReadOnlyStorage is readOnlyStorage on a workload placed on external capacity.
func externalReadOnlyStorage(w *v1.Workload) {
	externalPlacement(w)
	readOnlyStorage(w)
}

// readWriteWorkspace is a workspace with a read-write PFS volume at /ceph (user directories off)
// and a read-write hostpath volume at /data over /apps.
func readWriteWorkspace() *v1.Workspace {
	ws := jobutils.TestWorkspaceData.DeepCopy()
	ws.Spec.Volumes = []v1.WorkspaceVolume{{
		Id: 1, Type: v1.PFS, MountPath: "/ceph", AccessMode: corev1.ReadWriteMany,
		StorageClass: "storage-cephfs", Capacity: "100Gi",
	}, {
		Id: 2, Type: v1.HOSTPATH, MountPath: "/data", HostPath: "/apps", AccessMode: corev1.ReadWriteMany,
	}}
	return ws
}

// mountReadOnly returns, per container field ("containers", "initContainers"), the readOnly flag of
// every container mount whose volume is named in names, keyed by mount path. A mount path rendered
// with different flags in different containers of the same field fails the test.
func mountReadOnly(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload,
	spec v1.ResourceSpec, names func(string) bool) map[string]map[string]bool {
	t.Helper()
	result := map[string]map[string]bool{}
	for _, field := range []string{"containers", "initContainers"} {
		containers, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &spec, field))
		assert.NilError(t, err)
		result[field] = map[string]bool{}
		for _, ctr := range containers {
			c := ctr.(map[string]interface{})
			mounts, _ := c["volumeMounts"].([]interface{})
			for _, mount := range mounts {
				m := mount.(map[string]interface{})
				if !names(m["name"].(string)) {
					continue
				}
				path := m["mountPath"].(string)
				ro, _ := m["readOnly"].(bool)
				if prev, ok := result[field][path]; ok {
					assert.Equal(t, prev, ro, "%s %s", field, path)
				}
				result[field][path] = ro
			}
		}
	}
	return result
}

// workspaceOrHostPathVolume names the workspace volumes and spec.hostpath volumes of a rendered Pod.
func workspaceOrHostPathVolume(name string) bool {
	return name == "pfs-1" || strings.HasPrefix(name, "hostpath-")
}

// On the cluster's own nodes, every workspace volume of a workload with read-only workspace storage
// is mounted read-only into the main and the init containers, whatever its access mode, and so is
// a spec.hostpath entry under a workspace hostpath volume; an entry elsewhere keeps its mode.
// Without the annotation nothing changes.
func TestWorkspaceStorageReadOnlyOwnedPods(t *testing.T) {
	hostpaths := []string{"/apps/datasets", "/opt/extra"}
	for _, kind := range []string{common.PytorchJobKind, common.JobKind, common.DeploymentKind} {
		obj, workload, spec := renderWorkload(t, kind, readWriteWorkspace(), hostpaths, readOnlyStorage)
		got := mountReadOnly(t, obj, workload, spec, workspaceOrHostPathVolume)
		assert.DeepEqual(t, got["containers"], map[string]bool{
			"/ceph": true, "/data": true, "/apps/datasets": true, "/opt/extra": false,
		})
		if len(got["initContainers"]) > 0 {
			assert.DeepEqual(t, got["initContainers"], got["containers"])
		}

		obj, workload, spec = renderWorkload(t, kind, readWriteWorkspace(), hostpaths, nil)
		got = mountReadOnly(t, obj, workload, spec, workspaceOrHostPathVolume)
		assert.DeepEqual(t, got["containers"], map[string]bool{
			"/ceph": false, "/data": false, "/apps/datasets": false, "/opt/extra": false,
		})
	}
}

// A sandbox, whose template has init containers, gets the same read-only workspace mounts in them
// as in its main container.
func TestWorkspaceStorageReadOnlyInitContainers(t *testing.T) {
	obj, workload, spec := renderWorkload(t, common.SandboxKind, readWriteWorkspace(), nil, readOnlyStorage)
	got := mountReadOnly(t, obj, workload, spec, workspaceOrHostPathVolume)
	want := map[string]bool{"/ceph": true, "/data": true}
	assert.DeepEqual(t, got["containers"], want)
	assert.DeepEqual(t, got["initContainers"], want)

	obj, workload, spec = renderWorkload(t, common.SandboxKind, readWriteWorkspace(), nil, nil)
	got = mountReadOnly(t, obj, workload, spec, workspaceOrHostPathVolume)
	want = map[string]bool{"/ceph": false, "/data": false}
	assert.DeepEqual(t, got["containers"], want)
	assert.DeepEqual(t, got["initContainers"], want)
}

// On the cluster's own nodes a volume with user directories is mounted only as the workload's
// own user directory; with read-only workspace storage that mount is read-only too.
func TestWorkspaceStorageReadOnlyOwnedUserDir(t *testing.T) {
	withUserDirMounts(t, true)
	for _, kind := range []string{common.SandboxKind, common.PytorchJobKind} {
		pod := renderPodMounts(t, kind, topModelsWorkspace(), nil, readOnlyStorage)
		for _, field := range []string{"containers", "initContainers"} {
			for path, m := range pod.mounts[field] {
				if m.HostPath == "/shared_nfs" {
					assert.DeepEqual(t, m, renderedMount{"/shared_nfs", "Directory", "users/user-1", true})
					assert.Equal(t, path, "/shared_nfs/users/user-1")
				}
			}
		}
		assert.Equal(t, pod.mounts["containers"]["/shared_nfs/users/user-1"].ReadOnly, true, kind)
	}
}

// An external Pod with user directories mounted for external Pods and read-only workspace storage
// gets no user directory and no USER_DATA_PATH, and every shared path read-only, in the main and
// the init containers; its Pod has no volume for the user directory either.
func TestWorkspaceStorageReadOnlyExternalUserDirMounts(t *testing.T) {
	withUserDirMounts(t, true)
	want := map[string]renderedMount{}
	for _, shared := range topModelsSharedPaths {
		path := "/shared_nfs/" + shared.Path
		want[path] = renderedMount{path, "Directory", "", true}
	}
	for _, kind := range []string{common.SandboxKind, common.PytorchJobKind} {
		obj, workload, spec := renderWorkload(t, kind, topModelsWorkspace(), nil, externalReadOnlyStorage)
		pod := summarizeHostPathMounts(t, obj, workload, spec)
		assert.DeepEqual(t, pod.mounts["containers"], want)
		if kind == common.SandboxKind {
			assert.DeepEqual(t, pod.mounts["initContainers"], want)
		}
		assert.Equal(t, pod.userDataPath, "", kind)

		volumes, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &spec, "volumes"))
		assert.NilError(t, err)
		for _, vol := range volumes {
			hp, ok := vol.(map[string]interface{})["hostPath"].(map[string]interface{})
			if ok {
				assert.Assert(t, hp["path"] != "/shared_nfs/users/user-1", kind)
			}
		}
	}
}

// Without user directory mounts for external Pods, an external Pod with read-only workspace
// storage mounts the whole volume read-only.
func TestWorkspaceStorageReadOnlyExternalWholeVolume(t *testing.T) {
	withUserDirMounts(t, false)
	want := map[string]renderedMount{"/shared_nfs": {"/shared_nfs", "Directory", "", true}}
	pod := renderPodMounts(t, common.SandboxKind, topModelsWorkspace(), nil, externalReadOnlyStorage)
	for _, field := range []string{"containers", "initContainers"} {
		assert.DeepEqual(t, pod.mounts[field], want)
	}
}

// externalUserDirMounts leaves the user directory out and makes every shared path read-only for a
// workload with read-only workspace storage, and only for it.
func TestExternalUserDirMountsReadOnlyStorage(t *testing.T) {
	withUserDirMounts(t, true)
	vol := topModelsWorkspace().Spec.Volumes[0]
	workload := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w",
		Labels: map[string]string{v1.UserIdLabel: "user-1"}}}
	externalPlacement(workload)

	mounts, replaced, err := externalUserDirMounts(workload, vol)
	assert.NilError(t, err)
	assert.Assert(t, replaced)
	assert.Equal(t, len(mounts), 1+len(topModelsSharedPaths))
	assert.Equal(t, mounts[0].mountPath, "/shared_nfs/users/user-1")
	assert.Equal(t, mounts[0].readOnly, false)

	readOnlyStorage(workload)
	mounts, replaced, err = externalUserDirMounts(workload, vol)
	assert.NilError(t, err)
	assert.Assert(t, replaced)
	assert.Equal(t, len(mounts), len(topModelsSharedPaths))
	for _, m := range mounts {
		assert.Assert(t, m.mountPath != "/shared_nfs/users/user-1")
		assert.Assert(t, m.readOnly, m.mountPath)
	}

	// "false" is the same as no annotation.
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.WorkspaceStorageReadOnlyAnnotation, "false")
	mounts, _, err = externalUserDirMounts(workload, vol)
	assert.NilError(t, err)
	assert.Equal(t, len(mounts), 1+len(topModelsSharedPaths))
}

// A workload that does not use workspace storage mounts no workspace volume, with or without
// read-only workspace storage.
func TestWorkspaceStorageReadOnlyWithStorageOff(t *testing.T) {
	obj, workload, spec := renderWorkload(t, common.PytorchJobKind, readWriteWorkspace(), nil, func(w *v1.Workload) {
		readOnlyStorage(w)
		metav1.SetMetaDataAnnotation(&w.ObjectMeta, v1.UseWorkspaceStorageAnnotation, "false")
	})
	got := mountReadOnly(t, obj, workload, spec, workspaceOrHostPathVolume)
	assert.Equal(t, len(got["containers"]), 0)
}
