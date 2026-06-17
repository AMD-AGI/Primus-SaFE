/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workspace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func wsScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	return s
}

func wsWith(name string, vols ...v1.WorkspaceVolume) *v1.Workspace {
	w := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	w.Spec.Volumes = vols
	return w
}

func TestGetVolumeMountPathByPreference(t *testing.T) {
	ws := wsWith("ws1",
		v1.WorkspaceVolume{Type: v1.PFS, MountPath: "/wekafs"},
		v1.WorkspaceVolume{Type: v1.HOSTPATH, HostPath: "/data"},
	)
	assert.Equal(t, "", GetVolumeMountPathByPreference(ws, ""))
	assert.Equal(t, "/wekafs", GetVolumeMountPathByPreference(ws, "/wekafs"))
	// matches via HostPath when MountPath empty
	assert.Equal(t, "/data", GetVolumeMountPathByPreference(ws, "/data"))
	assert.Equal(t, "", GetVolumeMountPathByPreference(ws, "/nope"))
}

func TestResolveDownloadRoot(t *testing.T) {
	ws := wsWith("ws1",
		v1.WorkspaceVolume{Type: v1.PFS, MountPath: "/wekafs"},
		v1.WorkspaceVolume{Type: v1.HOSTPATH, HostPath: "/data"},
	)
	// preference hit
	assert.Equal(t, "/data", ResolveDownloadRoot(ws, "/data"))
	// fallback to PFS default
	assert.Equal(t, "/wekafs", ResolveDownloadRoot(ws, "/nope"))
}

func TestGetWorkspacesWithSamePath(t *testing.T) {
	a := wsWith("a", v1.WorkspaceVolume{Type: v1.PFS, MountPath: "/wekafs"})
	b := wsWith("b", v1.WorkspaceVolume{Type: v1.PFS, MountPath: "/wekafs"})
	c := wsWith("c", v1.WorkspaceVolume{Type: v1.PFS, MountPath: "/other"})
	cl := ctrlfake.NewClientBuilder().WithScheme(wsScheme(t)).WithObjects(a, b, c).Build()

	got, err := GetWorkspacesWithSamePath(cl, "/wekafs")
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, got)
}
