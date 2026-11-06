/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// privilegedHostPathWorkspace is sandboxHostPathWorkspace with /shared_nfs/admin, a hostpath volume
// for every scope that only the holders of workload/privileged get (requiresPrivilegedGrant).
func privilegedHostPathWorkspace() *v1.Workspace {
	ws := sandboxHostPathWorkspace()
	ws.Spec.Volumes = append(ws.Spec.Volumes, v1.WorkspaceVolume{
		Id: 4, Type: v1.HOSTPATH, MountPath: "/admin", HostPath: "/shared_nfs/admin", RequiresPrivilegedGrant: true,
	})
	return ws
}

// TestValidateHostPathRefusesPrivilegedVolumes verifies the webhook refuses, with the reason, a
// workload without the apiserver's hostpath grant mark that names a volume requiring the grant, a
// path under it or a path containing it, although /shared_nfs declares them; the privileged-volume
// mark alone does not admit it; the hostpath grant mark does; and a sandbox is refused whatever it
// carries.
func TestValidateHostPathRefusesPrivilegedVolumes(t *testing.T) {
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(newScheme(t)).
		WithObjects(privilegedHostPathWorkspace()).Build()}
	ctx := context.Background()

	for _, refused := range []string{"/shared_nfs/admin", "/shared_nfs/admin/keys", "/shared_nfs"} {
		job := jobWithHostPath(common.AuthoringKind, refused)
		v1.SetAnnotation(job, v1.WorkloadPrivilegedVolumesAuthzAnnotation, v1.TrueStr)
		err := v.validateHostPath(ctx, job, nil)
		assert.ErrorContains(t, err, `the hostpath "`+refused+`" reaches volume 4`)
		assert.ErrorContains(t, err, "requiresPrivilegedGrant")
		assert.ErrorContains(t, err, "requires the workload/privileged grant")

		sbx := sandboxWorkload(refused)
		v1.SetAnnotation(sbx, v1.WorkloadHostPathAuthzAnnotation, v1.TrueStr)
		assert.ErrorContains(t, v.validateSandbox(ctx, sbx, nil), "a sandbox cannot mount it")
	}
	assert.NilError(t, v.validateHostPath(ctx, jobWithHostPath(common.AuthoringKind, "/shared_nfs/models"), nil))

	granted := jobWithHostPath(common.AuthoringKind, "/shared_nfs/admin/keys")
	v1.SetAnnotation(granted, v1.WorkloadHostPathAuthzAnnotation, v1.TrueStr)
	assert.NilError(t, v.validateHostPath(ctx, granted, nil))
}

// TestMutateHostPathKeepsUnmountedPrivilegedVolume verifies an entry duplicating a volume that
// requires the grant is dropped as inherited only when the workload is marked to get the volume;
// otherwise it stays and is judged like any other entry.
func TestMutateHostPathKeepsUnmountedPrivilegedVolume(t *testing.T) {
	m := &WorkloadMutator{}
	ws := privilegedHostPathWorkspace()

	job := jobWithHostPath(common.PytorchJobKind, "/shared_nfs", "/shared_nfs/admin")
	m.mutateHostPath(job, ws)
	assert.DeepEqual(t, job.Spec.Hostpath, []string{"/shared_nfs/admin"})

	marked := jobWithHostPath(common.PytorchJobKind, "/shared_nfs", "/shared_nfs/admin")
	v1.SetAnnotation(marked, v1.WorkloadPrivilegedVolumesAuthzAnnotation, v1.TrueStr)
	m.mutateHostPath(marked, ws)
	assert.DeepEqual(t, marked.Spec.Hostpath, []string{})
}

// TestValidateVolumesRefusesOverlapWithPrivilegedVolume verifies a workspace cannot have an
// ordinary hostpath volume whose mounted directory equals, contains or lies under the hostPath of a
// volume requiring the grant, since it would hand the protected directory to every member; volumes
// beside it, a subPath that leads away from it, or both volumes requiring the grant are accepted.
func TestValidateVolumesRefusesOverlapWithPrivilegedVolume(t *testing.T) {
	v := &WorkspaceValidator{}
	ws := func(vols ...v1.WorkspaceVolume) *v1.Workspace {
		return &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: vols}}
	}
	protected := v1.WorkspaceVolume{Id: 1, Type: v1.HOSTPATH, HostPath: "/shared/secret", MountPath: "/secret",
		RequiresPrivilegedGrant: true}
	ordinary := func(hostPath, subPath string) v1.WorkspaceVolume {
		return v1.WorkspaceVolume{Id: 2, Type: v1.HOSTPATH, HostPath: hostPath, SubPath: subPath, MountPath: "/data"}
	}
	for _, o := range []v1.WorkspaceVolume{
		ordinary("/shared", ""), ordinary("/shared/secret", ""), ordinary("/shared/secret/keys", ""),
		ordinary("/", ""), ordinary("/shared", "secret"),
	} {
		err := v.validateVolumes(ws(protected, o), nil)
		assert.ErrorContains(t, err, "requiresPrivilegedGrant")
		err = v.validateVolumes(ws(o, protected), nil)
		assert.ErrorContains(t, err, "requiresPrivilegedGrant")
	}
	for _, o := range []v1.WorkspaceVolume{ordinary("/shared/other", ""), ordinary("/shared", "team"),
		ordinary("/shared/secretive", "")} {
		assert.NilError(t, v.validateVolumes(ws(protected, o), nil))
	}
	both := ordinary("/shared", "")
	both.RequiresPrivilegedGrant = true
	assert.NilError(t, v.validateVolumes(ws(protected, both), nil))
}
