/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"strings"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestWorkspaceValidateVolumeScopes verifies that volume scopes must be known scopes listed once,
// and that no scopes stays valid.
func TestWorkspaceValidateVolumeScopes(t *testing.T) {
	v := &WorkspaceValidator{}
	withScopes := func(scopes ...v1.WorkspaceScope) *v1.Workspace {
		return &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{{
			Id: 1, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc", Capacity: "100Gi",
			AccessMode: corev1.ReadWriteMany, Scopes: scopes,
		}}}}
	}
	assert.NilError(t, v.validateVolumes(withScopes(), nil))
	assert.NilError(t, v.validateVolumes(withScopes(v1.AuthoringScope, v1.CICDScope, v1.TrainScope,
		v1.InferScope, v1.RayScope), nil))

	err := v.validateVolumes(withScopes(v1.TrainScope, "Sandboxes"), nil)
	assert.ErrorContains(t, err, `invalid scope "Sandboxes" of volume(pfs-1)`)
	err = v.validateVolumes(withScopes(v1.TrainScope, v1.TrainScope), nil)
	assert.ErrorContains(t, err, `duplicate scope "Train" of volume(pfs-1)`)

	hostpath := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{{
		Id: 2, Type: v1.HOSTPATH, MountPath: "/data", HostPath: "/data", Scopes: []v1.WorkspaceScope{"train"},
	}}}}
	assert.ErrorContains(t, v.validateVolumes(hostpath, nil), `invalid scope "train" of volume(hostpath-2)`)
}

func sandboxWorkload(hostpath ...string) *v1.Workload {
	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "sbx"},
		Spec: v1.WorkloadSpec{
			Workspace:        "ws1",
			GroupVersionKind: v1.GroupVersionKind{Kind: common.SandboxKind, Version: common.DefaultVersion},
			Hostpath:         hostpath,
		},
	}
	return w
}

func sandboxHostPathWorkspace() *v1.Workspace {
	return &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.HOSTPATH, MountPath: "/shared_nfs", HostPath: "/shared_nfs"},
		{Id: 2, Type: v1.HOSTPATH, MountPath: "/apps", HostPath: "/apps",
			Scopes: []v1.WorkspaceScope{v1.TrainScope, v1.AuthoringScope}},
		{Id: 3, Type: v1.PFS, MountPath: "/mnt/ceph", StorageClass: "sc", Capacity: "1Ti"},
	}}}
}

// TestValidateSandboxHostPath verifies a sandbox may bind only the host paths its workspace
// declares for the Sandbox scope, or directories under them, and that the rule is enforced through
// validateCommon, the path every create and update takes.
func TestValidateSandboxHostPath(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(sandboxHostPathWorkspace()).Build()}
	ctx := context.Background()

	for _, allowed := range [][]string{
		nil,
		{"/shared_nfs"},
		{"/shared_nfs/models"},
		{"/shared_nfs/models/", "/shared_nfs/forge"},
	} {
		assert.NilError(t, v.validateSandbox(ctx, sandboxWorkload(allowed...), nil), "hostpath %v", allowed)
	}
	for _, refused := range [][]string{
		{"/"},
		{"/etc"},
		{"/shared_nfs_other"},
		{"/shared_nfs/../etc"},
		{"/shared_nfs/models/.."},
		{"shared_nfs"},
		{"/apps"},      // declared only for Train and Authoring
		{"/apps/data"}, // under a volume of other scopes
		{"/mnt/ceph"},  // a PFS volume is not a host path
		{"/shared_nfs/ok", "/var/run/docker.sock"},
	} {
		err := v.validateSandbox(ctx, sandboxWorkload(refused...), nil)
		assert.ErrorContains(t, err, "is not declared by workspace ws1")
	}

	// The same refusal through validateCommon, for creation and for a changed hostpath.
	err := v.validateCommon(ctx, sandboxWorkload("/etc"), nil)
	assert.ErrorContains(t, err, `the hostpath "/etc" of the sandbox is not declared`)
	err = v.validateCommon(ctx, sandboxWorkload("/etc"), sandboxWorkload("/shared_nfs"))
	assert.ErrorContains(t, err, `the hostpath "/etc" of the sandbox is not declared`)

	// An update that leaves the hostpath as it was is not re-judged, so a sandbox admitted before
	// a workspace change can still be updated (for example to remove its finalizer).
	assert.NilError(t, v.validateSandbox(ctx, sandboxWorkload("/etc"), sandboxWorkload("/etc")))

	// A sandbox whose workspace cannot be read is refused, not admitted with its hostpath unchecked.
	missing := sandboxWorkload("/shared_nfs")
	missing.Spec.Workspace = "nope"
	assert.ErrorContains(t, v.validateSandbox(ctx, missing, nil), `"nope" not found`)
	assert.ErrorContains(t, validateSandboxHostPath(missing, nil), "is not declared by workspace nope")

	// Other kinds are not restricted by this rule.
	job := sandboxWorkload("/etc")
	job.Spec.Kind = common.PytorchJobKind
	err = v.validateCommon(ctx, job, nil)
	assert.Assert(t, err == nil || !strings.Contains(err.Error(), "of the sandbox is not declared"))
}

// TestMutateHostPathRespectsVolumeScopes verifies a hostpath duplicating a workspace volume is
// dropped only when that volume is mounted into the workload's scope.
func TestMutateHostPathRespectsVolumeScopes(t *testing.T) {
	m := &WorkloadMutator{}
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.HOSTPATH, MountPath: "/shared_nfs", HostPath: "/shared_nfs"},
		{Id: 2, Type: v1.HOSTPATH, MountPath: "/apps", HostPath: "/apps",
			Scopes: []v1.WorkspaceScope{v1.TrainScope}},
	}}}

	sbx := sandboxWorkload("/shared_nfs", "/apps")
	m.mutateHostPath(sbx, ws)
	assert.DeepEqual(t, sbx.Spec.Hostpath, []string{"/apps"})

	job := sandboxWorkload("/shared_nfs", "/apps")
	job.Spec.Kind = common.PytorchJobKind
	m.mutateHostPath(job, ws)
	assert.DeepEqual(t, job.Spec.Hostpath, []string{})
}
