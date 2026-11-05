/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"strings"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// everyScopeButSandbox is the scope list the CPU/dev workspaces give their shared volume.
var everyScopeButSandbox = []v1.WorkspaceScope{
	v1.AuthoringScope, v1.CICDScope, v1.TrainScope, v1.InferScope, v1.RayScope,
}

// scopedWorkspace is a workspace with a shared PFS volume (pfs-1, user directories enabled)
// restricted to the given scopes, and an unrestricted hostpath volume (hostpath-2).
func scopedWorkspace(scopes []v1.WorkspaceScope) *v1.Workspace {
	ws := jobutils.TestWorkspaceData.DeepCopy()
	ws.Spec.Volumes = []v1.WorkspaceVolume{{
		Id: 1, Type: v1.PFS, MountPath: "/ceph", EnableUserDir: true,
		StorageClass: "storage-cephfs", Capacity: "100Gi", Scopes: scopes,
	}, {
		Id: 2, Type: v1.HOSTPATH, MountPath: "/data", HostPath: "/apps",
	}}
	return ws
}

// renderedVolumes renders the workload through generateK8sObject, the dispatcher's create path,
// and returns the pod-level volume names, every container mount path keyed by volume name, and
// whether any container carries USER_DATA_PATH.
func renderedVolumes(t *testing.T, kind string, workspace *v1.Workspace) (map[string]bool, map[string]string, bool) {
	t.Helper()
	obj, workload, spec := renderWorkload(t, kind, workspace, []string{"/opt/extra"}, nil)
	return summarizeVolumes(t, obj, workload, spec)
}

// renderWorkload renders a workload of the given kind with the given hostpaths through
// generateK8sObject and returns the object, the workload and the resource spec it was rendered
// with. mutate, when set, adjusts the workload before rendering.
func renderWorkload(t *testing.T, kind string, workspace *v1.Workspace, hostpaths []string,
	mutate func(*v1.Workload)) (*unstructured.Unstructured, *v1.Workload, v1.ResourceSpec) {
	t.Helper()
	obj, workload, spec, err := renderWorkloadErr(t, kind, workspace, hostpaths, mutate)
	assert.NilError(t, err)
	return obj, workload, spec
}

// renderWorkloadErr is renderWorkload that returns the generateK8sObject error instead of failing.
func renderWorkloadErr(t *testing.T, kind string, workspace *v1.Workspace, hostpaths []string,
	mutate func(*v1.Workload)) (*unstructured.Unstructured, *v1.Workload, v1.ResourceSpec, error) {
	t.Helper()
	c := activeDeadlineCases[kind]
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Workspace = workspace.Name
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: c.kind}
	if c.kind == common.DeploymentKind {
		workload.Spec.GroupVersionKind.Group = "apps"
	}
	if c.kind == common.SandboxKind {
		workload.Spec.GroupVersionKind.Version = common.DefaultVersion
		workload.Spec.JobPort = 0
		workload.Spec.EntryPoints = nil
		workload.Spec.Resources[0].RdmaResource = ""
		workload.Spec.Env[sandboxAuthPublicKeyEnvName] = "test-public-key"
	}
	workload.Spec.Hostpath = hostpaths
	if mutate != nil {
		mutate(workload)
	}
	metav1.SetMetaDataLabel(&workload.ObjectMeta, v1.UserIdLabel, "user-1")
	configmap, err := parseConfigmap(c.template)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap, c.rt, workspace).WithScheme(scheme).Build()
	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	return obj, workload, c.rt.Spec.ResourceSpecs[0], err
}

func summarizeVolumes(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload,
	spec v1.ResourceSpec) (map[string]bool, map[string]string, bool) {
	t.Helper()
	volumes, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &spec, "volumes"))
	assert.NilError(t, err)
	volumeNames := map[string]bool{}
	for _, vol := range volumes {
		name, _ := vol.(map[string]interface{})["name"].(string)
		volumeNames[name] = true
	}
	containers, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &spec, "containers"))
	assert.NilError(t, err)
	mounts := map[string]string{}
	userDataPath := false
	for _, ctr := range containers {
		m := ctr.(map[string]interface{})
		for _, mount := range m["volumeMounts"].([]interface{}) {
			mm := mount.(map[string]interface{})
			mounts[mm["name"].(string)] = mm["mountPath"].(string)
		}
		envs, _ := m["env"].([]interface{})
		for _, env := range envs {
			if env.(map[string]interface{})["name"] == jobutils.UserDataEnv {
				userDataPath = true
			}
		}
	}
	return volumeNames, mounts, userDataPath
}

// A Sandbox in a workspace whose shared volume excludes the Sandbox scope gets neither the
// volume, its mount nor USER_DATA_PATH, although it asks for workspace storage; the other
// volume and its own hostpath are untouched and keep their names.
func TestScopedWorkspaceVolumeExcludedFromSandbox(t *testing.T) {
	volumes, mounts, userDataPath := renderedVolumes(t, common.SandboxKind, scopedWorkspace(everyScopeButSandbox))
	assert.Assert(t, !volumes["pfs-1"], "pfs-1 volume rendered for a Sandbox: %v", volumes)
	_, mounted := mounts["pfs-1"]
	assert.Assert(t, !mounted, "pfs-1 mounted into a Sandbox: %v", mounts)
	assert.Assert(t, !userDataPath, "USER_DATA_PATH injected into a Sandbox")
	assert.Assert(t, volumes["hostpath-2"])
	assert.Equal(t, mounts["hostpath-2"], "/data")
	assert.Assert(t, volumes["hostpath-3"])
	assert.Equal(t, mounts["hostpath-3"], "/opt/extra")
	for name := range mounts {
		if isWorkspaceVolumeName(name) {
			assert.Assert(t, volumes[name], "mount %s has no volume", name)
		}
	}
}

// Kinds of the listed scopes still get the scoped volume through the same create path.
func TestScopedWorkspaceVolumeKeptForListedScopes(t *testing.T) {
	for _, kind := range []string{common.PytorchJobKind, common.JobKind, common.DeploymentKind} {
		t.Run(kind, func(t *testing.T) {
			volumes, mounts, userDataPath := renderedVolumes(t, kind, scopedWorkspace(everyScopeButSandbox))
			assert.Assert(t, volumes["pfs-1"], "pfs-1 volume missing for %s: %v", kind, volumes)
			assert.Equal(t, mounts["pfs-1"], "/ceph/users/user-1")
			assert.Assert(t, userDataPath)
			assert.Assert(t, volumes["hostpath-3"])
		})
	}
}

// A volume without scopes is mounted into a Sandbox exactly as before.
func TestUnscopedWorkspaceVolumeKeptForSandbox(t *testing.T) {
	volumes, mounts, userDataPath := renderedVolumes(t, common.SandboxKind, scopedWorkspace(nil))
	assert.Assert(t, volumes["pfs-1"])
	assert.Equal(t, mounts["pfs-1"], "/ceph/users/user-1")
	assert.Assert(t, userDataPath)
}

func isWorkspaceVolumeName(name string) bool {
	return strings.HasPrefix(name, "pfs-") || strings.HasPrefix(name, "hostpath-")
}

// Every kind maps to its scope: the scoped volume reaches Authoring, CICD, Train, Infer and Ray
// kinds and never a Sandbox; with no scopes every kind gets it.
func TestWorkspaceVolumeScopesPerKind(t *testing.T) {
	kinds := map[string]bool{
		common.AuthoringKind:           true,
		common.CICDScaleRunnerSetKind:  true,
		common.CICDEphemeralRunnerKind: true,
		common.CICDGithubRunnerKind:    true,
		common.PytorchJobKind:          true,
		common.JobKind:                 true,
		common.DeploymentKind:          true,
		common.InferaDeploymentKind:    true,
		common.RayJobKind:              true,
		common.SandboxKind:             false,
	}
	for kind, want := range kinds {
		for _, scopes := range [][]v1.WorkspaceScope{everyScopeButSandbox, nil} {
			expected := want || scopes == nil
			workload := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
				Name:        "w",
				Labels:      map[string]string{v1.UserIdLabel: "user-1"},
				Annotations: map[string]string{v1.UseWorkspaceStorageAnnotation: v1.TrueStr},
			}}
			workload.Spec.GroupVersionKind.Kind = kind
			ws := scopedWorkspace(scopes)

			mounted := false
			for _, m := range buildPersistentVolumeMounts(workload, ws) {
				if m.(map[string]interface{})["name"] == "pfs-1" {
					mounted = true
				}
			}
			assert.Equal(t, mounted, expected, "mount, kind %s scopes %v", kind, scopes)

			obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
			assert.NilError(t, modifyVolumes(obj, workload, ws, []string{"volumes"}))
			hasVolume := false
			vols, _, _ := jobutils.NestedSlice(obj.Object, []string{"volumes"})
			for _, v := range vols {
				if v.(map[string]interface{})["name"] == "pfs-1" {
					hasVolume = true
				}
			}
			assert.Equal(t, hasVolume, expected, "volume, kind %s scopes %v", kind, scopes)

			hasEnv := false
			for _, env := range buildEnvironment(workload, ws, 0) {
				if env.(map[string]interface{})["name"] == jobutils.UserDataEnv {
					hasEnv = true
				}
			}
			assert.Equal(t, hasEnv, expected, "USER_DATA_PATH, kind %s scopes %v", kind, scopes)

			wantNfs := "/data"
			if expected {
				wantNfs = "/ceph"
			}
			assert.Equal(t, getNfsPathFromWorkspace(workload, ws), wantNfs, "nfs path, kind %s scopes %v", kind, scopes)
		}
	}
}

// A GitHub runner keeps its state on a writable volume of its own scope, never on one restricted
// to other scopes.
func TestGithubRunnerSkipsVolumeOfOtherScopes(t *testing.T) {
	workload := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "runner-wl",
		Labels:      map[string]string{v1.UserIdLabel: "user-1"},
		Annotations: map[string]string{v1.UseWorkspaceStorageAnnotation: v1.TrueStr},
	}}
	workload.Spec.GroupVersionKind.Kind = common.CICDGithubRunnerKind
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/ceph", Scopes: []v1.WorkspaceScope{v1.TrainScope}},
		{Id: 2, Type: v1.HOSTPATH, MountPath: "/data", HostPath: "/apps"},
	}}}
	path, err := githubRunnerWritableMountPath(workload, ws)
	assert.NilError(t, err)
	assert.Equal(t, path, "/data")
	assert.Assert(t, !workspaceHasWritablePFS(workload, ws))

	ws.Spec.Volumes[0].Scopes = append(ws.Spec.Volumes[0].Scopes, v1.CICDScope)
	path, err = githubRunnerWritableMountPath(workload, ws)
	assert.NilError(t, err)
	assert.Equal(t, path, "/ceph")
	assert.Assert(t, workspaceHasWritablePFS(workload, ws))
}

// sandboxRootWorkspace declares /data/shared as the hostpath root for sandboxes (hostpath-1) and
// /data/train as a hostpath root for training only (hostpath-2).
func sandboxRootWorkspace() *v1.Workspace {
	ws := jobutils.TestWorkspaceData.DeepCopy()
	ws.Spec.Volumes = []v1.WorkspaceVolume{{
		Id: 1, Type: v1.HOSTPATH, MountPath: "/shared", HostPath: "/data/shared",
		Scopes: []v1.WorkspaceScope{v1.SandboxScope},
	}, {
		Id: 2, Type: v1.HOSTPATH, MountPath: "/train", HostPath: "/data/train",
		Scopes: []v1.WorkspaceScope{v1.TrainScope},
	}}
	return ws
}

// renderedHostPaths renders the workload and returns, per volume name, the hostPath source of the
// pod volume and the subPath of every container and init container mount of it.
func renderedHostPaths(t *testing.T, kind string, hostpaths []string, mutate func(*v1.Workload)) (
	map[string]string, map[string][]string) {
	t.Helper()
	obj, workload, spec := renderWorkload(t, kind, sandboxRootWorkspace(), hostpaths, mutate)
	volumes, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &spec, "volumes"))
	assert.NilError(t, err)
	sources := map[string]string{}
	for _, vol := range volumes {
		v := vol.(map[string]interface{})
		if hp, ok := v["hostPath"].(map[string]interface{}); ok {
			sources[v["name"].(string)] = hp["path"].(string)
		}
	}
	subPaths := map[string][]string{}
	for _, field := range []string{"containers", "initContainers"} {
		containers, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &spec, field))
		assert.NilError(t, err)
		for _, ctr := range containers {
			mounts, _ := ctr.(map[string]interface{})["volumeMounts"].([]interface{})
			for _, mount := range mounts {
				mm := mount.(map[string]interface{})
				sub, _ := mm["subPath"].(string)
				subPaths[mm["name"].(string)] = append(subPaths[mm["name"].(string)], mm["mountPath"].(string)+"|"+sub)
			}
		}
	}
	return sources, subPaths
}

// A Sandbox hostpath under a declared sandbox root is mounted as that root plus a subPath, so the
// kubelet, not the lexical admission check, decides whether a symlink inside it escapes the root.
func TestSandboxHostPathMountedThroughDeclaredRoot(t *testing.T) {
	sources, mounts := renderedHostPaths(t, common.SandboxKind,
		[]string{"/data/shared/team/a", "/data/shared", "/opt/legacy"}, nil)
	assert.Equal(t, sources["hostpath-3"], "/data/shared")
	assert.Equal(t, sources["hostpath-4"], "/data/shared")
	assert.Equal(t, sources["hostpath-5"], "/opt/legacy")
	assert.Assert(t, len(mounts["hostpath-3"]) > 0)
	for _, m := range mounts["hostpath-3"] {
		assert.Equal(t, m, "/data/shared/team/a|team/a")
	}
	for _, m := range mounts["hostpath-4"] {
		assert.Equal(t, m, "/data/shared|")
	}
	// Admitted before roots were enforced: rendered as before so the sandbox keeps working.
	for _, m := range mounts["hostpath-5"] {
		assert.Equal(t, m, "/opt/legacy|")
	}
}

// A root declared only for other scopes is not a sandbox root, and other kinds and external
// sandboxes (whose provider refuses subPath; modifyVolumes refuses their entries below a root)
// keep the hostpath as the volume source.
func TestHostPathSourceUnchangedOutsideSandboxRoots(t *testing.T) {
	sources, mounts := renderedHostPaths(t, common.SandboxKind, []string{"/data/train/x"}, nil)
	assert.Equal(t, sources["hostpath-3"], "/data/train/x")
	for _, m := range mounts["hostpath-3"] {
		assert.Equal(t, m, "/data/train/x|")
	}

	sources, mounts = renderedHostPaths(t, common.PytorchJobKind, []string{"/data/shared/team/a"}, nil)
	assert.Equal(t, sources["hostpath-3"], "/data/shared/team/a")
	for _, m := range mounts["hostpath-3"] {
		assert.Equal(t, m, "/data/shared/team/a|")
	}

	externalSandbox := &v1.Workload{
		Spec:   v1.WorkloadSpec{GroupVersionKind: v1.GroupVersionKind{Kind: common.SandboxKind}},
		Status: v1.WorkloadStatus{ExternalExecution: &v1.WorkloadExternalExecution{ClaimId: "claim-1"}},
	}
	assert.Assert(t, isExternalWorkload(externalSandbox))
	src, sub := sandboxHostPathSource(externalSandbox, sandboxRootWorkspace(), "/data/shared/team/a")
	assert.Equal(t, src, "/data/shared/team/a")
	assert.Equal(t, sub, "")
}

// The longest declared root wins, ".." and relative entries are never split, and "/" as a root
// yields a subPath without a leading slash.
func TestSandboxHostPathSourceRoots(t *testing.T) {
	sandbox := &v1.Workload{Spec: v1.WorkloadSpec{GroupVersionKind: v1.GroupVersionKind{Kind: common.SandboxKind}}}
	ws := sandboxRootWorkspace()
	ws.Spec.Volumes = append(ws.Spec.Volumes, v1.WorkspaceVolume{
		Id: 3, Type: v1.HOSTPATH, HostPath: "/data/shared/team/",
	})
	cases := []struct{ in, src, sub string }{
		{"/data/shared/team/a/b", "/data/shared/team", "a/b"},
		{"/data/shared/other", "/data/shared", "other"},
		{"/data/sharedx", "/data/sharedx", ""},
		{"/data/shared/../../etc", "/data/shared/../../etc", ""},
		{"data/shared/a", "data/shared/a", ""},
	}
	for _, c := range cases {
		src, sub := sandboxHostPathSource(sandbox, ws, c.in)
		assert.Equal(t, src, c.src, c.in)
		assert.Equal(t, sub, c.sub, c.in)
	}
	ws.Spec.Volumes = []v1.WorkspaceVolume{{Id: 1, Type: v1.HOSTPATH, HostPath: "/"}}
	src, sub := sandboxHostPathSource(sandbox, ws, "/etc/x")
	assert.Equal(t, src, "/")
	assert.Equal(t, sub, "etc/x")
}

// An external Sandbox's provider refuses subPath, so an entry below a declared sandbox root could
// only be mounted as its own hostPath and a symlink under the root would reach any node path.
// Rendering refuses it; the declared root itself and entries under no root render as before.
func TestExternalSandboxHostPathBelowRootRefused(t *testing.T) {
	external := func(w *v1.Workload) {
		w.Status.ExternalExecution = &v1.WorkloadExternalExecution{ClaimId: "claim-1",
			Placements: []v1.WorkloadExternalPlacement{{UnitKey: "master/0", NodeName: "vk-1",
				CPUMillis: 1000, MemoryBytes: 1 << 30}}}
	}
	_, workload, _, err := renderWorkloadErr(t, common.SandboxKind, sandboxRootWorkspace(),
		[]string{"/data/shared/team/a"}, external)
	assert.Assert(t, isExternalWorkload(workload))
	assert.ErrorContains(t, err, `hostpath "/data/shared/team/a" of an external sandbox is below the declared root "/data/shared"`)

	_, _, _, err = renderWorkloadErr(t, common.SandboxKind, sandboxRootWorkspace(),
		[]string{"/data/shared/", "/opt/legacy"}, external)
	assert.NilError(t, err)

	// A non-external Sandbox with the same entry renders through root + subPath.
	_, _, _, err = renderWorkloadErr(t, common.SandboxKind, sandboxRootWorkspace(),
		[]string{"/data/shared/team/a"}, nil)
	assert.NilError(t, err)
	// Other kinds are not restricted by sandbox roots.
	assert.NilError(t, checkExternalSandboxHostPath(&v1.Workload{
		Spec:   v1.WorkloadSpec{GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind}},
		Status: v1.WorkloadStatus{ExternalExecution: &v1.WorkloadExternalExecution{ClaimId: "claim-1"}},
	}, sandboxRootWorkspace(), "/data/shared/team/a"))
}
