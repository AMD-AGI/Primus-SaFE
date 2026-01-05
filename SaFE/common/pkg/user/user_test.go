/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package user

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestIsRolesEqual tests role comparison
func TestIsRolesEqual(t *testing.T) {
	tests := []struct {
		name     string
		roles1   []v1.UserRole
		roles2   []v1.UserRole
		expected bool
	}{
		{
			name:     "equal roles",
			roles1:   []v1.UserRole{v1.SystemAdminRole, v1.DefaultRole},
			roles2:   []v1.UserRole{v1.DefaultRole, v1.SystemAdminRole},
			expected: true,
		},
		{
			name:     "different roles",
			roles1:   []v1.UserRole{v1.SystemAdminRole},
			roles2:   []v1.UserRole{v1.DefaultRole},
			expected: false,
		},
		{
			name:     "different length",
			roles1:   []v1.UserRole{v1.SystemAdminRole, v1.WorkspaceAdminRole},
			roles2:   []v1.UserRole{v1.SystemAdminRole},
			expected: false,
		},
		{
			name:     "both empty",
			roles1:   []v1.UserRole{},
			roles2:   []v1.UserRole{},
			expected: true,
		},
		{
			name:     "one empty",
			roles1:   []v1.UserRole{v1.SystemAdminRole},
			roles2:   []v1.UserRole{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRolesEqual(tt.roles1, tt.roles2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAddWorkspace tests adding workspaces to user
func TestAddWorkspace(t *testing.T) {
	tests := []struct {
		name       string
		user       *v1.User
		workspaces []string
		wantResult bool
		validate   func(*testing.T, *v1.User)
	}{
		{
			name: "add to empty user",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: make(map[string][]string),
				},
			},
			workspaces: []string{"ws1", "ws2"},
			wantResult: true,
			validate: func(t *testing.T, u *v1.User) {
				assert.Contains(t, u.Spec.Resources[common.UserWorkspaces], "ws1")
				assert.Contains(t, u.Spec.Resources[common.UserWorkspaces], "ws2")
			},
		},
		{
			name: "add to existing workspaces",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: map[string][]string{
						common.UserWorkspaces: {"ws1"},
					},
				},
			},
			workspaces: []string{"ws2", "ws3"},
			wantResult: true,
			validate: func(t *testing.T, u *v1.User) {
				assert.Len(t, u.Spec.Resources[common.UserWorkspaces], 3)
				assert.Contains(t, u.Spec.Resources[common.UserWorkspaces], "ws1")
				assert.Contains(t, u.Spec.Resources[common.UserWorkspaces], "ws2")
				assert.Contains(t, u.Spec.Resources[common.UserWorkspaces], "ws3")
			},
		},
		{
			name: "add duplicate workspace",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: map[string][]string{
						common.UserWorkspaces: {"ws1", "ws2"},
					},
				},
			},
			workspaces: []string{"ws1", "ws3"},
			wantResult: true,
			validate: func(t *testing.T, u *v1.User) {
				assert.Len(t, u.Spec.Resources[common.UserWorkspaces], 3)
			},
		},
		{
			name:       "nil user",
			user:       nil,
			workspaces: []string{"ws1"},
			wantResult: false,
			validate:   nil,
		},
		{
			name: "empty workspaces",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: make(map[string][]string),
				},
			},
			workspaces: []string{},
			wantResult: false,
			validate:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AddWorkspace(tt.user, tt.workspaces...)
			assert.Equal(t, tt.wantResult, result)
			if tt.validate != nil {
				tt.validate(t, tt.user)
			}
		})
	}
}

// TestAddManagedWorkspace tests adding managed workspaces
func TestAddManagedWorkspace(t *testing.T) {
	tests := []struct {
		name       string
		user       *v1.User
		workspaces []string
		wantResult bool
	}{
		{
			name: "add managed workspaces",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: make(map[string][]string),
				},
			},
			workspaces: []string{"managed-ws1", "managed-ws2"},
			wantResult: true,
		},
		{
			name: "add to existing managed workspaces",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: map[string][]string{
						common.UserManagedWorkspaces: {"managed-ws1"},
					},
				},
			},
			workspaces: []string{"managed-ws2"},
			wantResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AddManagedWorkspace(tt.user, tt.workspaces...)
			assert.Equal(t, tt.wantResult, result)
			if result {
				managedWs := GetManagedWorkspace(tt.user)
				for _, ws := range tt.workspaces {
					assert.Contains(t, managedWs, ws)
				}
			}
		})
	}
}

// TestRemoveWorkspace tests removing workspaces
func TestRemoveWorkspace(t *testing.T) {
	tests := []struct {
		name       string
		user       *v1.User
		workspace  string
		wantResult bool
		validate   func(*testing.T, *v1.User)
	}{
		{
			name: "remove existing workspace",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: map[string][]string{
						common.UserWorkspaces: {"ws1", "ws2", "ws3"},
					},
				},
			},
			workspace:  "ws2",
			wantResult: true,
			validate: func(t *testing.T, u *v1.User) {
				assert.Len(t, u.Spec.Resources[common.UserWorkspaces], 2)
				assert.NotContains(t, u.Spec.Resources[common.UserWorkspaces], "ws2")
			},
		},
		{
			name: "remove non-existing workspace",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: map[string][]string{
						common.UserWorkspaces: {"ws1"},
					},
				},
			},
			workspace:  "ws-not-exist",
			wantResult: false,
			validate: func(t *testing.T, u *v1.User) {
				assert.Len(t, u.Spec.Resources[common.UserWorkspaces], 1)
			},
		},
		{
			name:       "nil user",
			user:       nil,
			workspace:  "ws1",
			wantResult: false,
			validate:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveWorkspace(tt.user, tt.workspace)
			assert.Equal(t, tt.wantResult, result)
			if tt.validate != nil {
				tt.validate(t, tt.user)
			}
		})
	}
}

// TestHasWorkspaceRight tests workspace access rights
func TestHasWorkspaceRight(t *testing.T) {
	tests := []struct {
		name       string
		user       *v1.User
		workspaces []string
		expected   bool
	}{
		{
			name: "has all rights",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: map[string][]string{
						common.UserWorkspaces: {"ws1", "ws2", "ws3"},
					},
				},
			},
			workspaces: []string{"ws1", "ws2"},
			expected:   true,
		},
		{
			name: "missing some workspaces",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: map[string][]string{
						common.UserWorkspaces: {"ws1"},
					},
				},
			},
			workspaces: []string{"ws1", "ws2"},
			expected:   false,
		},
		{
			name: "single workspace check",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: map[string][]string{
						common.UserWorkspaces: {"ws1", "ws2"},
					},
				},
			},
			workspaces: []string{"ws1"},
			expected:   true,
		},
		{
			name:       "nil user",
			user:       nil,
			workspaces: []string{"ws1"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasWorkspaceRight(tt.user, tt.workspaces...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAssignWorkspace tests workspace assignment
func TestAssignWorkspace(t *testing.T) {
	tests := []struct {
		name       string
		user       *v1.User
		workspaces []string
		validate   func(*testing.T, *v1.User)
	}{
		{
			name: "assign to user with existing workspaces",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: map[string][]string{
						common.UserWorkspaces: {"old-ws1", "old-ws2"},
					},
				},
			},
			workspaces: []string{"new-ws1", "new-ws2"},
			validate: func(t *testing.T, u *v1.User) {
				assert.Len(t, u.Spec.Resources[common.UserWorkspaces], 2)
				assert.Contains(t, u.Spec.Resources[common.UserWorkspaces], "new-ws1")
				assert.Contains(t, u.Spec.Resources[common.UserWorkspaces], "new-ws2")
				assert.NotContains(t, u.Spec.Resources[common.UserWorkspaces], "old-ws1")
			},
		},
		{
			name: "assign empty list",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: map[string][]string{
						common.UserWorkspaces: {"ws1"},
					},
				},
			},
			workspaces: []string{},
			validate: func(t *testing.T, u *v1.User) {
				_, exists := u.Spec.Resources[common.UserWorkspaces]
				assert.False(t, exists)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AssignWorkspace(tt.user, tt.workspaces...)
			tt.validate(t, tt.user)
		})
	}
}

// TestGenerateUserIdByName tests user ID generation
func TestGenerateUserIdByName(t *testing.T) {
	tests := []struct {
		name     string
		username string
		validate func(*testing.T, string)
	}{
		{
			name:     "normal username",
			username: "john.doe",
			validate: func(t *testing.T, id string) {
				assert.NotEmpty(t, id)
				assert.Len(t, id, 32) // MD5 produces 32 hex chars
			},
		},
		{
			name:     "email-like username",
			username: "user@example.com",
			validate: func(t *testing.T, id string) {
				assert.NotEmpty(t, id)
				assert.Len(t, id, 32)
			},
		},
		{
			name:     "consistent hashing",
			username: "testuser",
			validate: func(t *testing.T, id string) {
				id2 := GenerateUserIdByName("testuser")
				assert.Equal(t, id, id2, "Same username should produce same ID")
			},
		},
		{
			name:     "different usernames produce different IDs",
			username: "user1",
			validate: func(t *testing.T, id string) {
				id2 := GenerateUserIdByName("user2")
				assert.NotEqual(t, id, id2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateUserIdByName(tt.username)
			tt.validate(t, result)
		})
	}
}

// TestGetWorkspace tests getting user workspaces
func TestGetWorkspace(t *testing.T) {
	tests := []struct {
		name     string
		user     *v1.User
		expected []string
	}{
		{
			name: "user with workspaces",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: map[string][]string{
						common.UserWorkspaces: {"ws1", "ws2"},
					},
				},
			},
			expected: []string{"ws1", "ws2"},
		},
		{
			name: "user without workspaces",
			user: &v1.User{
				Spec: v1.UserSpec{
					Resources: make(map[string][]string),
				},
			},
			expected: nil,
		},
		{
			name:     "nil user",
			user:     nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetWorkspace(tt.user)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDelWorkspace tests deleting all workspaces
func TestDelWorkspace(t *testing.T) {
	user := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "test-user"},
		Spec: v1.UserSpec{
			Resources: map[string][]string{
				common.UserWorkspaces:        {"ws1", "ws2"},
				common.UserManagedWorkspaces: {"mws1"},
			},
		},
	}

	// Delete regular workspaces
	DelWorkspace(user)
	_, exists := user.Spec.Resources[common.UserWorkspaces]
	assert.False(t, exists, "Workspaces should be deleted")

	// Managed workspaces should still exist
	_, managedExists := user.Spec.Resources[common.UserManagedWorkspaces]
	assert.True(t, managedExists, "Managed workspaces should still exist")

	// Delete managed workspaces
	DelManagedWorkspace(user)
	_, managedExists = user.Spec.Resources[common.UserManagedWorkspaces]
	assert.False(t, managedExists, "Managed workspaces should be deleted")
}
