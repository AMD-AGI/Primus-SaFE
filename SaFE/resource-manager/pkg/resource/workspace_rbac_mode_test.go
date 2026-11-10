/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

func TestProjectedGroupMappingsFollowTheMode(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("auth.group_mappings", []map[string]any{{"group": "admins", "roles": []string{"system-admin"}}})

	got, err := projectedGroupMappings()
	require.NoError(t, err)
	require.Len(t, got, 1)

	viper.Set("auth.mode", "builtin")
	got, err = projectedGroupMappings()
	require.NoError(t, err)
	require.Empty(t, got)

	viper.Set("auth.mode", "bultin")
	_, err = projectedGroupMappings()
	require.Error(t, err)
}

func TestRoleChangeDoesNotChangeProjectedSubjects(t *testing.T) {
	old := &v1.User{}
	old.Spec.Roles = []v1.UserRole{v1.DefaultRole}
	commonuser.AssignWorkspace(old, "lab")
	next := old.DeepCopy()
	next.Spec.Roles = []v1.UserRole{v1.SystemAdminRole}
	require.False(t, isUserRBACChanged(old, next))

	commonuser.AssignWorkspace(next, "prod")
	require.True(t, isUserRBACChanged(old, next))
}
