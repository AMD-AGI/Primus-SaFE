/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"os"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apiextensionsinternal "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func workspaceWithSharedPaths(enableUserDir bool, paths ...string) *v1.Workspace {
	vol := v1.WorkspaceVolume{
		Id: 1, Type: v1.HOSTPATH, MountPath: "/shared_nfs", HostPath: "/shared_nfs", EnableUserDir: enableUserDir,
	}
	for _, p := range paths {
		vol.SharedPaths = append(vol.SharedPaths, v1.WorkspaceSharedPath{Path: p})
	}
	return &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{vol}}}
}

// Shared paths of a hostpath volume are clean relative paths, listed once, and outside the user
// directories when the volume has them; no shared paths stays valid.
func TestWorkspaceValidateVolumeSharedPaths(t *testing.T) {
	v := &WorkspaceValidator{}
	assert.NilError(t, v.validateVolumes(workspaceWithSharedPaths(true), nil))
	assert.NilError(t, v.validateVolumes(workspaceWithSharedPaths(true,
		"hyperloom-claw", "hf_cache", "hyperloom/models", "hyperloom-claw/benchmark-configs", "usersx"), nil))
	// Without user directories, "users" is an ordinary directory.
	assert.NilError(t, v.validateVolumes(workspaceWithSharedPaths(false, "users/shared"), nil))

	refused := map[string]string{
		"":             `a shared path of volume(hostpath-1) is empty`,
		"/models":      `shared path "/models" of volume(hostpath-1) must be relative`,
		".":            `shared path "." of volume(hostpath-1) must be a clean path`,
		"models/":      `shared path "models/" of volume(hostpath-1) must be a clean path`,
		"a//b":         `shared path "a//b" of volume(hostpath-1) must be a clean path`,
		"a/../models":  `shared path "a/../models" of volume(hostpath-1) must be a clean path`,
		"..":           `shared path ".." of volume(hostpath-1) has a ".." element`,
		"../etc":       `shared path "../etc" of volume(hostpath-1) has a ".." element`,
		"users":        `shared path "users" of volume(hostpath-1) is in the user directories`,
		"users/abc123": `shared path "users/abc123" of volume(hostpath-1) is in the user directories`,
	}
	for p, msg := range refused {
		assert.ErrorContains(t, v.validateVolumes(workspaceWithSharedPaths(true, "models", p), nil), msg, p)
	}
	assert.ErrorContains(t, v.validateVolumes(workspaceWithSharedPaths(true, "models", "hf_cache", "models"), nil),
		`duplicate shared path "models" of volume(hostpath-1)`)

	pfs := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{{
		Id: 2, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc", Capacity: "100Gi",
		AccessMode: corev1.ReadWriteMany, SharedPaths: []v1.WorkspaceSharedPath{{Path: "models"}},
	}}}}
	assert.ErrorContains(t, v.validateVolumes(pfs, nil),
		`volume(pfs-2) lists sharedPaths, which only a hostpath volume can have`)
}

func workspaceObjectWithSharedPaths(sharedPaths ...interface{}) map[string]interface{} {
	obj := workspaceWithVolumeScopes()
	volume := obj["spec"].(map[string]interface{})["volumes"].([]interface{})[0].(map[string]interface{})
	volume["type"], volume["hostPath"] = "hostpath", "/shared_nfs"
	volume["sharedPaths"] = sharedPaths
	return obj
}

// workspaceCRDStructural is the structural schema of the chart's Workspace CRD, the one the API
// server prunes unknown fields with.
func workspaceCRDStructural(t *testing.T) *structuralschema.Structural {
	t.Helper()
	raw, err := os.ReadFile("../../charts/primus-safe/crds/amd.com_workspaces.yaml")
	assert.NilError(t, err)
	crd := &apiextensionsv1.CustomResourceDefinition{}
	assert.NilError(t, yaml.Unmarshal(raw, crd))
	scheme := runtime.NewScheme()
	assert.NilError(t, apiextensionsinternal.AddToScheme(scheme))
	assert.NilError(t, apiextensionsv1.AddToScheme(scheme))
	internal := &apiextensionsinternal.CustomResourceValidation{}
	assert.NilError(t, scheme.Convert(crd.Spec.Versions[0].Schema, internal, nil))
	structural, err := structuralschema.NewStructural(internal.OpenAPIV3Schema)
	assert.NilError(t, err)
	return structural
}

// The chart's CRD keeps sharedPaths (the API server would otherwise prune it), requires a path,
// and refuses a path listed twice.
func TestWorkspaceCRDVolumeSharedPaths(t *testing.T) {
	validate := workspaceCRDValidate(t)

	valid := workspaceObjectWithSharedPaths(
		map[string]interface{}{"path": "hyperloom-claw"},
		map[string]interface{}{"path": "models", "readOnly": true},
	)
	assert.Equal(t, len(validate(valid)), 0)
	pruning.Prune(valid, workspaceCRDStructural(t), true)
	volume := valid["spec"].(map[string]interface{})["volumes"].([]interface{})[0].(map[string]interface{})
	assert.DeepEqual(t, volume["sharedPaths"], []interface{}{
		map[string]interface{}{"path": "hyperloom-claw"},
		map[string]interface{}{"path": "models", "readOnly": true},
	})

	errs := validate(workspaceObjectWithSharedPaths(map[string]interface{}{"readOnly": true}))
	assert.Assert(t, len(errs) > 0)
	assert.ErrorContains(t, errs.ToAggregate(), "spec.volumes[0].sharedPaths[0].path")

	errs = validate(workspaceObjectWithSharedPaths(
		map[string]interface{}{"path": "models"},
		map[string]interface{}{"path": "models", "readOnly": true},
	))
	assert.Assert(t, len(errs) > 0)
	assert.ErrorContains(t, errs.ToAggregate(), "Duplicate value")
}
