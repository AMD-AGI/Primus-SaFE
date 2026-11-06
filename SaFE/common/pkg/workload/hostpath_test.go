/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func hostPathRootsWorkspace() *v1.Workspace {
	return &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.HOSTPATH, HostPath: "/shared_nfs"},
		{Id: 2, Type: v1.HOSTPATH, HostPath: "/shared_nfs/team/", Scopes: []v1.WorkspaceScope{v1.TrainScope}},
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
		got := DeclaredHostPathRoot(ws, c.scope, c.in)
		assert.Equal(t, got.Root, c.root, "%s %q", c.scope, c.in)
		assert.Equal(t, got.Sub, c.sub, "%s %q", c.scope, c.in)
		assert.Equal(t, got.ReadOnly, false, "%s %q", c.scope, c.in)
	}

	assert.Equal(t, DeclaredHostPathRoot(nil, v1.TrainScope, "/shared_nfs"), DeclaredHostPath{})
	slash := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{{Type: v1.HOSTPATH, HostPath: "/"}}}}
	assert.Equal(t, DeclaredHostPathRoot(slash, v1.TrainScope, "/etc/x"), DeclaredHostPath{Root: "/", Sub: "etc/x"})
}

func TestUndeclaredHostPaths(t *testing.T) {
	ws := hostPathRootsWorkspace()
	got := UndeclaredHostPaths(ws, v1.TrainScope, []string{"/shared_nfs/a", "/etc", "/apps", "/shared_nfs/team"})
	assert.DeepEqual(t, got, []string{"/etc", "/apps"})
	assert.Assert(t, UndeclaredHostPaths(ws, v1.TrainScope, nil) == nil)
	assert.DeepEqual(t, UndeclaredHostPaths(nil, v1.TrainScope, []string{"/shared_nfs"}), []string{"/shared_nfs"})
}

// exposedPartWorkspace has a volume that shows only its subPath, read-only (hostpath-1), a volume
// with user directories (hostpath-2) and one with a subPath (hostpath-9), volumes whose subPath
// leaves hostPath (hostpath-3, -4), nested volumes with different access modes (hostpath-5 to -8),
// volumes whose hostPath has ".." or is relative (hostpath-10, -11), and volumes whose hostPath has
// a trailing or a leading space (hostpath-12, -13).
func exposedPartWorkspace() *v1.Workspace {
	return &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.HOSTPATH, HostPath: "/shared", SubPath: "teamA", AccessMode: corev1.ReadOnlyMany},
		{Id: 2, Type: v1.HOSTPATH, HostPath: "/home", EnableUserDir: true},
		{Id: 3, Type: v1.HOSTPATH, HostPath: "/bad", SubPath: "../etc"},
		{Id: 4, Type: v1.HOSTPATH, HostPath: "/abs", SubPath: "/etc"},
		{Id: 5, Type: v1.HOSTPATH, HostPath: "/data", AccessMode: corev1.ReadOnlyMany},
		{Id: 6, Type: v1.HOSTPATH, HostPath: "/data/rw", AccessMode: corev1.ReadWriteMany},
		{Id: 7, Type: v1.HOSTPATH, HostPath: "/models"},
		{Id: 8, Type: v1.HOSTPATH, HostPath: "/models/ro", AccessMode: corev1.ReadOnlyMany},
		{Id: 9, Type: v1.HOSTPATH, HostPath: "/nfs", SubPath: "site", EnableUserDir: true,
			AccessMode: corev1.ReadOnlyMany},
		{Id: 10, Type: v1.HOSTPATH, HostPath: "/var/run/../tmp"},
		{Id: 11, Type: v1.HOSTPATH, HostPath: "relative/dir"},
		{Id: 12, Type: v1.HOSTPATH, HostPath: "/srv/share "},
		{Id: 13, Type: v1.HOSTPATH, HostPath: " /srv/lead"},
	}}}
}

// A volume declares only what it mounts: hostPath joined with subPath, nothing for a subPath that
// leaves hostPath, and with user directories neither the volume's directory itself nor anything
// in or over its user directories. Each directory directly under a volume with user directories is
// a root of its own, so the kubelet keeps a symlink in it from reaching the user directories. An
// entry is read-only when every root declaring it is; a writable volume over or under a read-only
// one shows the entry writable already.
func TestDeclaredHostPathRootOnlyWhatTheVolumeShows(t *testing.T) {
	ws := exposedPartWorkspace()
	for _, c := range []struct {
		in   string
		want DeclaredHostPath
	}{
		{"/shared/teamA", DeclaredHostPath{Root: "/shared/teamA", ReadOnly: true}},
		{"/shared/teamA/x/", DeclaredHostPath{Root: "/shared/teamA", Sub: "x", ReadOnly: true}},
		{"/shared", DeclaredHostPath{}},
		{"/shared/teamB", DeclaredHostPath{}},
		{"/shared/teamAB", DeclaredHostPath{}},
		{"/home", DeclaredHostPath{}},
		{"/home/", DeclaredHostPath{}},
		{"/home/users", DeclaredHostPath{}},
		{"/home/users/u1", DeclaredHostPath{}},
		{"/home/users/u2/x", DeclaredHostPath{}},
		{"/home/models", DeclaredHostPath{Root: "/home/models"}},
		{"/home/hyperloom/runner-cache", DeclaredHostPath{Root: "/home/hyperloom", Sub: "runner-cache"}},
		{"/home/usersX/a", DeclaredHostPath{Root: "/home/usersX", Sub: "a"}},
		{"/nfs/site/models/m", DeclaredHostPath{Root: "/nfs/site/models", Sub: "m", ReadOnly: true}},
		{"/nfs/site", DeclaredHostPath{}},
		{"/nfs/site/users/u1", DeclaredHostPath{}},
		{"/nfs/other", DeclaredHostPath{}},
		{"/bad", DeclaredHostPath{}},
		// The node resolves /var/run/../tmp after following symlinks (to /tmp where /var/run links
		// to /run), so neither its lexical form nor anything else is declared for it.
		{"/var/tmp", DeclaredHostPath{}},
		{"/var/tmp/x", DeclaredHostPath{}},
		{"/tmp", DeclaredHostPath{}},
		{"/relative/dir", DeclaredHostPath{}},
		// A hostPath is used as the volume mounts it: "/srv/share " is not "/srv/share", and
		// " /srv/lead" is not absolute.
		{"/srv/share/secret", DeclaredHostPath{}},
		{"/srv/share", DeclaredHostPath{}},
		{"/srv/share /x", DeclaredHostPath{Root: "/srv/share ", Sub: "x"}},
		{"/srv/lead", DeclaredHostPath{}},
		{"/etc", DeclaredHostPath{}},
		{"/abs/etc", DeclaredHostPath{}},
		{"/data/x", DeclaredHostPath{Root: "/data", Sub: "x", ReadOnly: true}},
		{"/data/rw/x", DeclaredHostPath{Root: "/data/rw", Sub: "x"}},
		{"/models/x", DeclaredHostPath{Root: "/models", Sub: "x"}},
		{"/models/ro/x", DeclaredHostPath{Root: "/models/ro", Sub: "x"}},
	} {
		assert.Equal(t, DeclaredHostPathRoot(ws, v1.TrainScope, c.in), c.want, c.in)
	}
	roots := DeclaredHostPathRoots(ws, v1.TrainScope)
	assert.DeepEqual(t, roots, []HostPathRoot{
		{Path: "/shared/teamA", ReadOnly: true},
		{Path: "/home", UserDirs: true},
		{Path: "/data", ReadOnly: true},
		{Path: "/data/rw"},
		{Path: "/models"},
		{Path: "/models/ro", ReadOnly: true},
		{Path: "/nfs/site", ReadOnly: true, UserDirs: true},
		{Path: "/srv/share "},
	})
	assert.Equal(t, roots[0].String(), "/shared/teamA")
	assert.Equal(t, roots[1].String(), "/home/* but /home/users")
	assert.DeepEqual(t, UndeclaredHostPaths(ws, v1.TrainScope,
		[]string{"/shared/teamA/x", "/shared/teamB", "/home/users/u1", "/home/models"}),
		[]string{"/shared/teamB", "/home/users/u1"})

	// Two volumes with the same root: writable when either is.
	same := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.HOSTPATH, HostPath: "/same", AccessMode: corev1.ReadOnlyMany},
		{Id: 2, Type: v1.HOSTPATH, HostPath: "/same/"},
	}}}
	assert.Equal(t, DeclaredHostPathRoot(same, v1.TrainScope, "/same/x"), DeclaredHostPath{Root: "/same", Sub: "x"})
	same.Spec.Volumes[1].Scopes = []v1.WorkspaceScope{v1.InferScope}
	assert.Equal(t, DeclaredHostPathRoot(same, v1.TrainScope, "/same/x"),
		DeclaredHostPath{Root: "/same", Sub: "x", ReadOnly: true})
}

// HostPathVolumeRoot, which external Pods are held to, is the volume's hostPath whatever the
// volume declares below it.
func TestHostPathVolumeRoot(t *testing.T) {
	ws := exposedPartWorkspace()
	for _, c := range []struct{ in, root, sub string }{
		{"/shared/teamB", "/shared", "teamB"},
		{"/shared/teamA", "/shared", "teamA"},
		{"/home/users/u1", "/home", "users/u1"},
		{"/home/", "/home", ""},
		{"/data/rw", "/data/rw", ""},
		{"/data/rw/x", "/data/rw", "x"},
		{"/etc", "", ""},
		{"/home/../etc", "", ""},
		{"home/x", "", ""},
	} {
		root, sub := HostPathVolumeRoot(ws, v1.TrainScope, c.in)
		assert.Equal(t, root, c.root, c.in)
		assert.Equal(t, sub, c.sub, c.in)
	}
	root, sub := HostPathVolumeRoot(nil, v1.TrainScope, "/home")
	assert.Equal(t, root+sub, "")
}
