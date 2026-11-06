/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"testing"

	"gotest.tools/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func hostPathRootsWorkspace() *v1.Workspace {
	return &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.HOSTPATH, HostPath: "/shared_nfs"},
		{Id: 2, Type: v1.HOSTPATH, HostPath: " /shared_nfs/team/ ", Scopes: []v1.WorkspaceScope{v1.TrainScope}},
		{Id: 3, Type: v1.HOSTPATH, HostPath: "/apps", Scopes: []v1.WorkspaceScope{v1.SandboxScope}},
		{Id: 4, Type: v1.PFS, MountPath: "/mnt/ceph"},
		{Id: 5, Type: v1.HOSTPATH, HostPath: "  "},
	}}}
}

// The longest root declared for the scope wins; a root restricted to other scopes, a PFS volume
// and a blank root are not roots; "..", relative entries and look-alike prefixes are never under a
// root; a workload without a scope sees every root.
func TestDeclaredHostPathRoot(t *testing.T) {
	ws := hostPathRootsWorkspace()
	for _, c := range []struct {
		scope     v1.WorkspaceScope
		in        string
		root, sub string
	}{
		{v1.TrainScope, "/shared_nfs", "/shared_nfs", ""},
		{v1.TrainScope, "/shared_nfs/", "/shared_nfs", ""},
		{v1.TrainScope, "/shared_nfs//a/./b", "/shared_nfs", "a/b"},
		{v1.TrainScope, "/shared_nfs/team/x", "/shared_nfs/team", "x"},
		{v1.InferScope, "/shared_nfs/team/x", "/shared_nfs", "team/x"},
		{v1.TrainScope, "/apps/x", "", ""},
		{v1.SandboxScope, "/apps/x", "/apps", "x"},
		{"", "/apps/x", "/apps", "x"},
		{v1.TrainScope, "/mnt/ceph", "", ""},
		{v1.TrainScope, "/shared_nfs_other", "", ""},
		{v1.TrainScope, "/shared_nfs/../etc", "", ""},
		{v1.TrainScope, "/shared_nfs/a/..", "", ""},
		{v1.TrainScope, "shared_nfs/a", "", ""},
		{v1.TrainScope, "", "", ""},
		{v1.TrainScope, "/", "", ""},
	} {
		root, sub := DeclaredHostPathRoot(ws, c.scope, c.in)
		assert.Equal(t, root, c.root, "%s %q", c.scope, c.in)
		assert.Equal(t, sub, c.sub, "%s %q", c.scope, c.in)
	}

	root, sub := DeclaredHostPathRoot(nil, v1.TrainScope, "/shared_nfs")
	assert.Equal(t, root+sub, "")
	slash := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{{Type: v1.HOSTPATH, HostPath: "/"}}}}
	root, sub = DeclaredHostPathRoot(slash, v1.TrainScope, "/etc/x")
	assert.Equal(t, root, "/")
	assert.Equal(t, sub, "etc/x")
}

func TestUndeclaredHostPaths(t *testing.T) {
	ws := hostPathRootsWorkspace()
	got := UndeclaredHostPaths(ws, v1.TrainScope, []string{"/shared_nfs/a", "/etc", "/apps", "/shared_nfs/team"})
	assert.DeepEqual(t, got, []string{"/etc", "/apps"})
	assert.Assert(t, UndeclaredHostPaths(ws, v1.TrainScope, nil) == nil)
	assert.DeepEqual(t, UndeclaredHostPaths(nil, v1.TrainScope, []string{"/shared_nfs"}), []string{"/shared_nfs"})
}
