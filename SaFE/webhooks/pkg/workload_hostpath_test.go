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
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// jobWithHostPath is a workload of the given kind in workspace ws1 that asks for hostpath.
func jobWithHostPath(kind string, hostpath ...string) *v1.Workload {
	w := sandboxWorkload(hostpath...)
	w.Name = "job"
	w.Spec.Kind = kind
	return w
}

// TestValidateHostPathOfOtherKinds verifies a workload that is not a sandbox may name, without the
// grant, only a hostpath volume its workspace declares for the workload's scope or a directory
// under one; that a privileged workload or one the apiserver marked as granted is not restricted;
// and that an update is judged on the entries it adds.
func TestValidateHostPathOfOtherKinds(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(sandboxHostPathWorkspace()).Build()}
	ctx := context.Background()

	for _, allowed := range [][]string{
		nil,
		{"/shared_nfs"},
		{"/shared_nfs/models"},
		{"/shared_nfs/models/", "/shared_nfs/forge"},
		{"/apps"},      // declared for Train and Authoring
		{"/apps/data"}, // under it
	} {
		assert.NilError(t, v.validateHostPath(ctx, jobWithHostPath(common.PytorchJobKind, allowed...), nil),
			"hostpath %v", allowed)
		assert.NilError(t, v.validateHostPath(ctx, jobWithHostPath(common.AuthoringKind, allowed...), nil),
			"hostpath %v", allowed)
	}
	for _, refused := range [][]string{
		{"/"},
		{"/etc"},
		{"/root/.ssh"},
		{"/var/run/docker.sock"},
		{"/shared_nfs_other"},
		{"/shared_nfs/../etc"},
		{"/shared_nfs/models/.."},
		{"shared_nfs"},
		{"/mnt/ceph"}, // a PFS volume is not a host path
		{"/shared_nfs/ok", "/etc"},
	} {
		err := v.validateHostPath(ctx, jobWithHostPath(common.PytorchJobKind, refused...), nil)
		assert.ErrorContains(t, err, "is not declared by workspace ws1 for PyTorchJob workloads")
		assert.ErrorContains(t, err, "requires the workload/privileged grant")
	}
	// /apps is declared for Train and Authoring only, so an inference workload may not name it.
	for _, refused := range []string{"/apps", "/apps/data"} {
		err := v.validateHostPath(ctx, jobWithHostPath(common.DeploymentKind, refused), nil)
		assert.ErrorContains(t, err, `the hostpath "`+refused+`" is not declared by workspace ws1 for Deployment workloads`)
	}

	// The apiserver marks a workload whose creator holds workload/privileged in the workspace; a
	// privileged workload took that grant to be created.
	granted := jobWithHostPath(common.PytorchJobKind, "/etc", "/")
	v1.SetAnnotation(granted, v1.WorkloadHostPathAuthzAnnotation, v1.TrueStr)
	assert.NilError(t, v.validateHostPath(ctx, granted, nil))
	privileged := jobWithHostPath(common.PytorchJobKind, "/etc", "/")
	v1.SetAnnotation(privileged, v1.WorkloadPrivilegedAnnotation, v1.TrueStr)
	assert.NilError(t, v.validateHostPath(ctx, privileged, nil))
	notGranted := jobWithHostPath(common.PytorchJobKind, "/etc")
	v1.SetAnnotation(notGranted, v1.WorkloadHostPathAuthzAnnotation, "yes")
	assert.ErrorContains(t, v.validateHostPath(ctx, notGranted, nil), `the hostpath "/etc" is not declared`)

	// An update keeps what was admitted and is refused only for what it adds.
	admitted := jobWithHostPath(common.PytorchJobKind, "/etc")
	assert.NilError(t, v.validateHostPath(ctx, jobWithHostPath(common.PytorchJobKind, "/etc"), admitted))
	assert.NilError(t, v.validateHostPath(ctx, jobWithHostPath(common.PytorchJobKind), admitted))
	assert.NilError(t, v.validateHostPath(ctx, jobWithHostPath(common.PytorchJobKind, "/etc", "/shared_nfs/a"), admitted))
	err := v.validateHostPath(ctx, jobWithHostPath(common.PytorchJobKind, "/etc", "/root"), admitted)
	assert.ErrorContains(t, err, `the hostpath "/root" is not declared`)

	// A workspace that cannot be read refuses the hostpath instead of admitting it unchecked, and
	// a workload outside any workspace (an ops job in "default") has nothing declared.
	missing := jobWithHostPath(common.PytorchJobKind, "/shared_nfs")
	missing.Spec.Workspace = "nope"
	assert.ErrorContains(t, v.validateHostPath(ctx, missing, nil), `"nope" not found`)
	noWorkspace := jobWithHostPath(common.PytorchJobKind, "/shared_nfs")
	noWorkspace.Spec.Workspace = corev1.NamespaceDefault
	assert.ErrorContains(t, v.validateHostPath(ctx, noWorkspace, nil), "is not declared by workspace default")
}

// TestWorkloadAdmissionRefusesUndeclaredHostPath runs the rule through the validating webhook's
// entry point, as the apiserver's request reaches it, for creation and for updates.
func TestWorkloadAdmissionRefusesUndeclaredHostPath(t *testing.T) {
	ctx := context.Background()
	c := fullWorkloadEnvClient(t)
	ws := &v1.Workspace{}
	assert.NilError(t, c.Get(ctx, client.ObjectKey{Name: "ws1"}, ws))
	ws.Spec.Volumes = sandboxHostPathWorkspace().Spec.Volumes
	assert.NilError(t, c.Update(ctx, ws))
	v := &WorkloadValidator{Client: c, decoder: newDecoder(t)}

	withHostPath := func(hostpath ...string) *v1.Workload {
		w := fullValidWorkload()
		w.Spec.Hostpath = hostpath
		return w
	}

	resp := v.Handle(ctx, newRequest(t, admissionv1.Create, withHostPath("/shared_nfs/models"), nil))
	assert.Assert(t, resp.Allowed, "a declared hostpath: %v", resp.Result)

	resp = v.Handle(ctx, newRequest(t, admissionv1.Create, withHostPath("/"), nil))
	assert.Assert(t, !resp.Allowed)
	assert.Equal(t, resp.Result.Code, int32(403))
	assert.Assert(t, strings.Contains(resp.Result.Message, `the hostpath "/" is not declared by workspace ws1`),
		resp.Result.Message)

	granted := withHostPath("/")
	v1.SetAnnotation(granted, v1.WorkloadHostPathAuthzAnnotation, v1.TrueStr)
	resp = v.Handle(ctx, newRequest(t, admissionv1.Create, granted, nil))
	assert.Assert(t, resp.Allowed, "a granted hostpath: %v", resp.Result)

	resp = v.Handle(ctx, newRequest(t, admissionv1.Update, withHostPath("/"), withHostPath("/")))
	assert.Assert(t, resp.Allowed, "an update that keeps the hostpath: %v", resp.Result)
	resp = v.Handle(ctx, newRequest(t, admissionv1.Update, withHostPath("/shared_nfs", "/etc"), withHostPath("/shared_nfs")))
	assert.Assert(t, !resp.Allowed)
	assert.Assert(t, strings.Contains(resp.Result.Message, `the hostpath "/etc" is not declared by workspace ws1`),
		resp.Result.Message)
}

// exposedPartWorkspace is workspace ws1 with a volume that shows workloads only /shared/teamA,
// read-only, a volume with user directories over /home, and a volume whose hostPath has "..".
func exposedPartWorkspace() *v1.Workspace {
	return &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.HOSTPATH, MountPath: "/team", HostPath: "/shared", SubPath: "teamA",
			AccessMode: corev1.ReadOnlyMany},
		{Id: 2, Type: v1.HOSTPATH, MountPath: "/home", HostPath: "/home", EnableUserDir: true},
		{Id: 3, Type: v1.HOSTPATH, MountPath: "/scratch", HostPath: "/var/run/../tmp"},
	}}}
}

// TestValidateHostPathOnlyWhatTheVolumeShows verifies a volume declares, for every kind and for
// sandboxes, only what it mounts: a volume with a subPath declares hostPath/subPath and not its
// siblings or hostPath itself, and a volume with user directories declares the directories under it
// but neither its user directories (another user's or the user's own, which the volume mounts
// already) nor its own directory, which holds them. None of these is admitted without the grant.
func TestValidateHostPathOnlyWhatTheVolumeShows(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exposedPartWorkspace()).Build()}
	ctx := context.Background()

	for _, allowed := range []string{
		"/shared/teamA", "/shared/teamA/", "/shared/teamA/data",
		"/home/models", "/home/hyperloom/runner-cache",
	} {
		assert.NilError(t, v.validateHostPath(ctx, jobWithHostPath(common.PytorchJobKind, allowed), nil), allowed)
		assert.NilError(t, v.validateSandbox(ctx, sandboxWorkload(allowed), nil), allowed)
	}
	declared := "only paths under [/shared/teamA /home/* but /home/users] are allowed"
	for _, refused := range []string{
		"/shared",           // above the subPath
		"/shared/teamB",     // next to it
		"/shared/teamAB",    // a look-alike of it
		"/home",             // the directory of a volume with user directories
		"/home/users",       // every user's directory
		"/home/users/other", // another user's directory
		"/home/users/u1",    // the user's own, which the volume mounts already
		"/var/tmp",          // the lexical form of a hostPath with "..", which the node resolves elsewhere
	} {
		job := jobWithHostPath(common.PytorchJobKind, refused)
		v1.SetLabel(job, v1.UserIdLabel, "u1")
		err := v.validateHostPath(ctx, job, nil)
		assert.ErrorContains(t, err, `the hostpath "`+refused+`" is not declared by workspace ws1 for PyTorchJob workloads, `+
			declared)
		sandbox := sandboxWorkload(refused)
		v1.SetLabel(sandbox, v1.UserIdLabel, "u1")
		err = v.validateSandbox(ctx, sandbox, nil)
		assert.ErrorContains(t, err, `the hostpath "`+refused+`" of the sandbox is not declared by workspace ws1, `+
			declared)
	}
}
