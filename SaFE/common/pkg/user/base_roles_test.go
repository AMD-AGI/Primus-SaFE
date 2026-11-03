/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package user

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// setBaseRoles sets users.base_roles for one test; nil leaves the key unset.
func setBaseRoles(t *testing.T, value any) {
	t.Helper()
	viper.Set("users.base_roles", value)
	t.Cleanup(func() { viper.Set("users.base_roles", nil) })
}

func TestBaseRoles(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
		want  []v1.UserRole
	}{
		{"unset is the default role, as the platform always did", nil, []v1.UserRole{v1.DefaultRole}},
		{"an empty list is the default: a user cannot be created without a role", []string{}, []v1.UserRole{v1.DefaultRole}},
		{"blank names are dropped, leaving the default", []string{"", "  "}, []v1.UserRole{v1.DefaultRole}},
		{"the minimal role", []string{"self-read"}, []v1.UserRole{v1.SelfReadRole}},
		{"a comma separated scalar is accepted", "self-read,viewer", []v1.UserRole{v1.SelfReadRole, "viewer"}},
		{"order is kept and repeats are dropped", []string{"b", "a", "b"}, []v1.UserRole{"b", "a"}},
		{"names are trimmed", []string{" self-read "}, []v1.UserRole{v1.SelfReadRole}},
		{"the platform roles are never everyone's", []string{"system-admin", "system-admin-readonly", "self-read"}, []v1.UserRole{v1.SelfReadRole}},
		{"the workspace-admin marker is never a base role", []string{"workspace-admin", "self-read"}, []v1.UserRole{v1.SelfReadRole}},
		{"a list of nothing but refused names is the default", []string{"system-admin", "workspace-admin"}, []v1.UserRole{v1.DefaultRole}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setBaseRoles(t, tt.value)
			require.Equal(t, tt.want, BaseRoles())
		})
	}
}

func TestMissingBaseRoles(t *testing.T) {
	for _, tt := range []struct {
		name   string
		base   any
		stored []v1.UserRole
		want   []v1.UserRole
	}{
		{"default config, nothing held", nil, nil, []v1.UserRole{v1.DefaultRole}},
		{"default config, held", nil, []v1.UserRole{v1.DefaultRole}, nil},
		{"default config, another role held", nil, []v1.UserRole{"viewer"}, []v1.UserRole{v1.DefaultRole}},
		{"minimal config, a user of today", []string{"self-read"}, []v1.UserRole{v1.DefaultRole}, []v1.UserRole{v1.SelfReadRole}},
		{"minimal config, held", []string{"self-read"}, []v1.UserRole{v1.SelfReadRole}, nil},
		{"two base roles, one held", []string{"a", "b"}, []v1.UserRole{"b"}, []v1.UserRole{"a"}},
		{"two base roles, none held, in config order", []string{"a", "b"}, nil, []v1.UserRole{"a", "b"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setBaseRoles(t, tt.base)
			require.Equal(t, tt.want, MissingBaseRoles(tt.stored))
		})
	}
}
