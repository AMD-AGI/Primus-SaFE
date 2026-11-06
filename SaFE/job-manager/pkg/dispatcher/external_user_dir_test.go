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
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

const userDirMountsKey = "external_execution.user_dir_mounts"

// withUserDirMounts sets external_execution.user_dir_mounts for the test.
func withUserDirMounts(t *testing.T, on bool) {
	t.Helper()
	if on {
		commonconfig.SetValue(userDirMountsKey, "true")
	} else {
		commonconfig.SetValue(userDirMountsKey, "false")
	}
	t.Cleanup(func() { commonconfig.UnsetValue(userDirMountsKey) })
}

// topModelsSharedPaths are the directories, below /shared_nfs, that the agent sandboxes of the
// top-models workspace use today besides the user's own directory: the team session trees
// (resumed runs and release legs write there), the Hugging Face cache, Hyperloom's data root, and
// read-only model and source trees.
var topModelsSharedPaths = []v1.WorkspaceSharedPath{
	{Path: "hyperloom-claw"},
	{Path: "hf_cache"},
	{Path: "hyperloom-slurm"},
	{Path: "models", ReadOnly: true},
	{Path: "hyperloom/models", ReadOnly: true},
	{Path: "hyperloom/KernelForge", ReadOnly: true},
}

// topModelsWorkspace is a workspace whose Pods run on external nodes: one hostpath volume at
// /shared_nfs with user directories and the shared paths above.
func topModelsWorkspace() *v1.Workspace {
	ws := jobutils.TestWorkspaceData.DeepCopy()
	ws.Name = "top-models"
	ws.Spec.Volumes = []v1.WorkspaceVolume{{
		Id: 1, Type: v1.HOSTPATH, MountPath: "/shared_nfs", HostPath: "/shared_nfs",
		EnableUserDir: true, AccessMode: corev1.ReadWriteMany,
		SharedPaths: append([]v1.WorkspaceSharedPath(nil), topModelsSharedPaths...),
	}}
	return ws
}

// externalPlacement marks a workload as admitted against external capacity.
func externalPlacement(w *v1.Workload) {
	w.Status.ExternalExecution = &v1.WorkloadExternalExecution{ClaimId: "claim-1",
		Placements: []v1.WorkloadExternalPlacement{{UnitKey: "master/0", NodeName: "vk-1",
			CPUMillis: 1000, MemoryBytes: 1 << 30}}}
}

// renderedMount is one container mount of a rendered Pod with the hostPath source of its volume.
type renderedMount struct {
	HostPath string
	HostType string
	SubPath  string
	ReadOnly bool
}

// renderedPod is what a rendered Pod mounts: per container field ("containers", "initContainers"),
// the mounts of hostPath volumes keyed by mount path, and the USER_DATA_PATH of the containers.
type renderedPod struct {
	mounts       map[string]map[string]renderedMount
	userDataPath string
}

func summarizeHostPathMounts(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload,
	spec v1.ResourceSpec) renderedPod {
	t.Helper()
	volumes, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &spec, "volumes"))
	assert.NilError(t, err)
	type source struct{ path, kind string }
	sources := map[string]source{}
	for _, vol := range volumes {
		v := vol.(map[string]interface{})
		if hp, ok := v["hostPath"].(map[string]interface{}); ok {
			name := v["name"].(string)
			_, dup := sources[name]
			assert.Assert(t, !dup, "volume %s rendered twice", name)
			kind, _ := hp["type"].(string)
			sources[name] = source{hp["path"].(string), kind}
		}
	}
	pod := renderedPod{mounts: map[string]map[string]renderedMount{}}
	for _, field := range []string{"containers", "initContainers"} {
		containers, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &spec, field))
		assert.NilError(t, err)
		pod.mounts[field] = map[string]renderedMount{}
		for _, ctr := range containers {
			c := ctr.(map[string]interface{})
			mounts, _ := c["volumeMounts"].([]interface{})
			for _, mount := range mounts {
				mm := mount.(map[string]interface{})
				src, ok := sources[mm["name"].(string)]
				if !ok {
					continue
				}
				sub, _ := mm["subPath"].(string)
				ro, _ := mm["readOnly"].(bool)
				pod.mounts[field][mm["mountPath"].(string)] = renderedMount{src.path, src.kind, sub, ro}
			}
			envs, _ := c["env"].([]interface{})
			for _, env := range envs {
				e := env.(map[string]interface{})
				if e["name"] == jobutils.UserDataEnv {
					pod.userDataPath, _ = e["value"].(string)
				}
			}
		}
	}
	return pod
}

func renderPodMounts(t *testing.T, kind string, ws *v1.Workspace, hostpaths []string,
	mutate func(*v1.Workload)) renderedPod {
	t.Helper()
	obj, workload, spec := renderWorkload(t, kind, ws, hostpaths, mutate)
	return summarizeHostPathMounts(t, obj, workload, spec)
}

// An agent sandbox of the top-models workspace on an external node, with user directories mounted
// for external Pods, mounts its own user directory read-write at USER_DATA_PATH and each shared
// path at the same absolute path, read-write or read-only as configured, and nothing else of
// /shared_nfs: neither the root nor another user's directory. The envd init container sees the
// same mounts.
func TestExternalUserDirMountsTopModelsAgentSandbox(t *testing.T) {
	withUserDirMounts(t, true)
	pod := renderPodMounts(t, common.SandboxKind, topModelsWorkspace(), nil, externalPlacement)

	want := map[string]renderedMount{
		"/shared_nfs/users/user-1":          {"/shared_nfs/users/user-1", "DirectoryOrCreate", "", false},
		"/shared_nfs/hyperloom-claw":        {"/shared_nfs/hyperloom-claw", "Directory", "", false},
		"/shared_nfs/hf_cache":              {"/shared_nfs/hf_cache", "Directory", "", false},
		"/shared_nfs/hyperloom-slurm":       {"/shared_nfs/hyperloom-slurm", "Directory", "", false},
		"/shared_nfs/models":                {"/shared_nfs/models", "Directory", "", true},
		"/shared_nfs/hyperloom/models":      {"/shared_nfs/hyperloom/models", "Directory", "", true},
		"/shared_nfs/hyperloom/KernelForge": {"/shared_nfs/hyperloom/KernelForge", "Directory", "", true},
	}
	for _, field := range []string{"containers", "initContainers"} {
		assert.DeepEqual(t, pod.mounts[field], want)
	}
	assert.Equal(t, pod.userDataPath, "/shared_nfs/users/user-1")
}

// The same sandbox without the flag mounts /shared_nfs whole, as before: that is what the provider
// admits until it admits paths below an approved root.
func TestExternalUserDirMountsOffMountsTheWholeVolume(t *testing.T) {
	want := map[string]renderedMount{"/shared_nfs": {"/shared_nfs", "Directory", "", false}}
	withUserDirMounts(t, false)
	pod := renderPodMounts(t, common.SandboxKind, topModelsWorkspace(), nil, externalPlacement)
	for _, field := range []string{"containers", "initContainers"} {
		assert.DeepEqual(t, pod.mounts[field], want)
	}
	assert.Equal(t, pod.userDataPath, "/shared_nfs/users/user-1")

	// Unset reads as off.
	commonconfig.UnsetValue(userDirMountsKey)
	pod = renderPodMounts(t, common.SandboxKind, topModelsWorkspace(), nil, externalPlacement)
	assert.DeepEqual(t, pod.mounts["containers"], want)
}

// Pods on the cluster's own nodes are not affected by the flag: they mount the user directory
// through subPath of the volume root and no shared paths.
func TestExternalUserDirMountsLeaveInternalPodsAlone(t *testing.T) {
	withUserDirMounts(t, true)
	for _, kind := range []string{common.SandboxKind, common.PytorchJobKind} {
		pod := renderPodMounts(t, kind, topModelsWorkspace(), nil, nil)
		got := map[string]renderedMount{}
		for path, m := range pod.mounts["containers"] {
			if strings.HasPrefix(m.HostPath, "/shared_nfs") {
				got[path] = m
			}
		}
		want := map[string]renderedMount{
			"/shared_nfs/users/user-1": {"/shared_nfs", "Directory", "users/user-1", false},
		}
		assert.DeepEqual(t, got, want)
		assert.Equal(t, pod.userDataPath, "/shared_nfs/users/user-1")
	}
}

// Other external kinds get the same mounts as the sandbox, and a volume that has neither user
// directories nor shared paths is still mounted whole.
func TestExternalUserDirMountsOtherKindsAndPlainVolumes(t *testing.T) {
	withUserDirMounts(t, true)
	pod := renderPodMounts(t, common.PytorchJobKind, topModelsWorkspace(), nil, externalPlacement)
	assert.Equal(t, len(pod.mounts["containers"]), 1+len(topModelsSharedPaths))
	assert.Equal(t, pod.mounts["containers"]["/shared_nfs/users/user-1"].HostPath, "/shared_nfs/users/user-1")
	_, whole := pod.mounts["containers"]["/shared_nfs"]
	assert.Assert(t, !whole)

	plain := topModelsWorkspace()
	plain.Spec.Volumes[0].EnableUserDir = false
	plain.Spec.Volumes[0].SharedPaths = nil
	pod = renderPodMounts(t, common.SandboxKind, plain, nil, externalPlacement)
	assert.DeepEqual(t, pod.mounts["containers"],
		map[string]renderedMount{"/shared_nfs": {"/shared_nfs", "Directory", "", false}})
	assert.Equal(t, pod.userDataPath, "")

	// Shared paths without user directories: only the shared paths.
	sharedOnly := topModelsWorkspace()
	sharedOnly.Spec.Volumes[0].EnableUserDir = false
	pod = renderPodMounts(t, common.SandboxKind, sharedOnly, nil, externalPlacement)
	assert.Equal(t, len(pod.mounts["containers"]), len(topModelsSharedPaths))
	_, own := pod.mounts["containers"]["/shared_nfs/users/user-1"]
	assert.Assert(t, !own)
}

// A read-only volume stays read-only in every mount, and the volume's subPath is the root of both
// the user directory and the shared paths on the node.
func TestExternalUserDirMountsReadOnlyVolumeAndSubPath(t *testing.T) {
	withUserDirMounts(t, true)
	ws := topModelsWorkspace()
	ws.Spec.Volumes[0].AccessMode = corev1.ReadOnlyMany
	ws.Spec.Volumes[0].SubPath = "site"
	ws.Spec.Volumes[0].SharedPaths = []v1.WorkspaceSharedPath{{Path: "hf_cache"}}
	pod := renderPodMounts(t, common.SandboxKind, ws, nil, externalPlacement)
	assert.DeepEqual(t, pod.mounts["containers"], map[string]renderedMount{
		"/shared_nfs/users/user-1": {"/shared_nfs/site/users/user-1", "DirectoryOrCreate", "", true},
		"/shared_nfs/hf_cache":     {"/shared_nfs/site/hf_cache", "Directory", "", true},
	})
}

// A sandbox that does not use workspace storage (a measurement sandbox) gets no user directory and
// no USER_DATA_PATH, and the shared directories it names in spec.hostpath are mounted read-only,
// each as its own hostPath at the same path. Without the flag those entries are refused, as before.
func TestExternalUserDirMountsSandboxWithoutWorkspaceStorage(t *testing.T) {
	noStorage := func(w *v1.Workload) {
		externalPlacement(w)
		metav1.SetMetaDataAnnotation(&w.ObjectMeta, v1.UseWorkspaceStorageAnnotation, "false")
	}
	hostpaths := []string{"/shared_nfs/models", "/shared_nfs/hyperloom/runner-cache"}

	withUserDirMounts(t, true)
	pod := renderPodMounts(t, common.SandboxKind, topModelsWorkspace(), hostpaths, noStorage)
	assert.DeepEqual(t, pod.mounts["containers"], map[string]renderedMount{
		"/shared_nfs/models":                 {"/shared_nfs/models", "Directory", "", true},
		"/shared_nfs/hyperloom/runner-cache": {"/shared_nfs/hyperloom/runner-cache", "Directory", "", true},
	})
	assert.Equal(t, pod.userDataPath, "")

	withUserDirMounts(t, false)
	_, _, _, err := renderWorkloadErr(t, common.SandboxKind, topModelsWorkspace(), hostpaths, noStorage)
	assert.ErrorContains(t, err, `below the declared root "/shared_nfs"`)
}

// With the flag on, no external workload may name in spec.hostpath a path that equals, lies under
// or contains the user directories, or that is relative or has "..": each would reach other
// users' directories. Pods on the cluster's own nodes are not checked here.
func TestExternalUserDirMountsRefuseHostPathsOverUserDirs(t *testing.T) {
	withUserDirMounts(t, true)
	refused := map[string]string{
		"/shared_nfs/users/other":           "overlaps the user directories",
		"/shared_nfs/users":                 "overlaps the user directories",
		"/shared_nfs/":                      "overlaps the user directories",
		"/":                                 "overlaps the user directories",
		"/shared_nfs/models/../users/other": `has a ".." element`,
		"shared_nfs/models":                 "is not absolute",
	}
	for _, kind := range []string{common.SandboxKind, common.PytorchJobKind} {
		for hostpath, msg := range refused {
			_, _, _, err := renderWorkloadErr(t, kind, topModelsWorkspace(), []string{hostpath}, externalPlacement)
			assert.ErrorContains(t, err, msg, "%s %s", kind, hostpath)
		}
		_, _, _, err := renderWorkloadErr(t, kind, topModelsWorkspace(),
			[]string{"/shared_nfs/usersx/a", "/opt/data"}, externalPlacement)
		assert.NilError(t, err, kind)
	}
	// Not external: unchanged.
	assert.NilError(t, checkExternalUserDirHostPath(&v1.Workload{}, topModelsWorkspace(), "/shared_nfs/users/other"))
}

// The user directory needs the creator's user id, and a shared path is checked again when the
// Pod is rendered: a workspace written before the webhook checked it cannot widen a mount.
func TestExternalUserDirMountsRefuseBadInputs(t *testing.T) {
	withUserDirMounts(t, true)
	vol := topModelsWorkspace().Spec.Volumes[0]
	workload := &v1.Workload{}
	externalPlacement(workload)
	workload.Name = "w"
	_, replaced, err := externalUserDirMounts(workload, vol)
	assert.Assert(t, replaced)
	assert.ErrorContains(t, err, "no user id")

	workload.Labels = map[string]string{v1.UserIdLabel: "user-1"}
	for _, bad := range []string{"users/other", "../etc", "/etc", ".", "a/../users"} {
		v := vol
		v.SharedPaths = []v1.WorkspaceSharedPath{{Path: bad}}
		_, replaced, err = externalUserDirMounts(workload, v)
		assert.Assert(t, replaced)
		assert.Assert(t, err != nil, bad)
	}

	ws := topModelsWorkspace()
	ws.Spec.Volumes[0].SharedPaths = []v1.WorkspaceSharedPath{{Path: "users"}}
	_, _, _, err = renderWorkloadErr(t, common.SandboxKind, ws, nil, externalPlacement)
	assert.ErrorContains(t, err, `shared path "users" of volume(hostpath-1) is in the user directories`)
}

// Every mount has a pod volume and every volume name is used once, so the Pod passes the API
// server; mounts nested in another come after it.
func TestExternalUserDirMountsVolumesMatchMounts(t *testing.T) {
	withUserDirMounts(t, true)
	ws := topModelsWorkspace()
	ws.Spec.Volumes[0].SharedPaths = append(ws.Spec.Volumes[0].SharedPaths,
		v1.WorkspaceSharedPath{Path: "hyperloom-claw/benchmark-configs", ReadOnly: true})
	obj, workload, spec := renderWorkload(t, common.SandboxKind, ws, nil, externalPlacement)
	volumes, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &spec, "volumes"))
	assert.NilError(t, err)
	names := map[string]bool{}
	for _, vol := range volumes {
		name := vol.(map[string]interface{})["name"].(string)
		assert.Assert(t, !names[name], "volume %s twice", name)
		names[name] = true
	}
	containers, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &spec, "containers"))
	assert.NilError(t, err)
	var paths []string
	for _, mount := range containers[0].(map[string]interface{})["volumeMounts"].([]interface{}) {
		m := mount.(map[string]interface{})
		assert.Assert(t, names[m["name"].(string)], "mount %v has no volume", m)
		paths = append(paths, m["mountPath"].(string))
	}
	parent, child := -1, -1
	for i, p := range paths {
		switch p {
		case "/shared_nfs/hyperloom-claw":
			parent = i
		case "/shared_nfs/hyperloom-claw/benchmark-configs":
			child = i
		}
	}
	assert.Assert(t, parent >= 0 && child > parent, "mount order %v", paths)
}
