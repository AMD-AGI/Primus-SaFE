/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workspace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGetNfsPathFromWorkspace(t *testing.T) {
	tests := []struct {
		name      string
		workspace *v1.Workspace
		expected  string
	}{
		{
			name: "workspace with PFS volume",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{
							Type:      v1.PFS,
							MountPath: "/pfs/data",
						},
					},
				},
			},
			expected: "/pfs/data",
		},
		{
			name: "workspace with multiple volumes - PFS prioritized",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{
							Type:      v1.HOSTPATH,
							MountPath: "/hostpath/data",
						},
						{
							Type:      v1.PFS,
							MountPath: "/pfs/data",
						},
					},
				},
			},
			expected: "/pfs/data",
		},
		{
			name: "workspace with only hostpath volume",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{
							Type:      v1.HOSTPATH,
							MountPath: "/hostpath/data",
						},
					},
				},
			},
			expected: "/hostpath/data",
		},
		{
			name: "workspace with no volumes",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{},
				},
			},
			expected: "",
		},
		{
			name:      "nil workspace volumes",
			workspace: &v1.Workspace{},
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetNfsPathFromWorkspace(tt.workspace)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUniqueDownloadPaths(t *testing.T) {
	tests := []struct {
		name        string
		workspaces  []v1.Workspace
		expectedLen int
	}{
		{
			name: "single workspace",
			workspaces: []v1.Workspace{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/ws1"},
						},
					},
				},
			},
			expectedLen: 1,
		},
		{
			name: "multiple workspaces with different paths",
			workspaces: []v1.Workspace{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/ws1"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-2"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/ws2"},
						},
					},
				},
			},
			expectedLen: 2,
		},
		{
			name: "multiple workspaces with same path - deduplicated",
			workspaces: []v1.Workspace{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/shared"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-2"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/shared"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-3"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/shared"},
						},
					},
				},
			},
			expectedLen: 1,
		},
		{
			name: "workspaces with no volumes are skipped",
			workspaces: []v1.Workspace{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/ws1"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-2"},
					Spec:       v1.WorkspaceSpec{},
				},
			},
			expectedLen: 1,
		},
		{
			name:        "empty workspaces",
			workspaces:  []v1.Workspace{},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUniqueDownloadPaths(tt.workspaces)
			assert.Equal(t, tt.expectedLen, len(result))
		})
	}
}
