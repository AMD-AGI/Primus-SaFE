/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"testing"

	"github.com/spf13/viper"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestApplyMappedRolesDropsRolesTheNewGroupsDoNotGrant checks that a role some mapping can grant
// is removed when the new groups do not grant it, and that a role no mapping grants stays.
func TestApplyMappedRolesDropsRolesTheNewGroupsDoNotGrant(t *testing.T) {
	viper.Set("auth.group_mappings", []map[string]any{
		{"group": "leads", "roles": []string{"workspace-admin"}},
	})
	t.Cleanup(func() { viper.Set("auth.group_mappings", nil) })

	user := &v1.User{Spec: v1.UserSpec{Roles: []v1.UserRole{"workspace-admin", "default"}}}
	changed, err := applyMappedRoles(user, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("roles did not change")
	}
	if len(user.Spec.Roles) != 1 || user.Spec.Roles[0] != "default" {
		t.Fatalf("roles %v, want [default]", user.Spec.Roles)
	}
}
