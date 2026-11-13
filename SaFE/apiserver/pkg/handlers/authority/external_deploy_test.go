/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// External mode needs a login URL, a group that grants system-admin, and a resolvable SCIM base while SCIM is enabled.
func TestCheckExternalDeployment(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	require.NoError(t, CheckExternalDeployment())

	viper.Set("auth.mode", "external")
	require.ErrorContains(t, CheckExternalDeployment(), "login_url")

	viper.Set("auth.login_url", "https://idp.example/sign-in")
	require.ErrorContains(t, CheckExternalDeployment(), "system-admin")

	viper.Set("auth.group_mappings", []map[string]any{{"group": "admins", "roles": []string{"system-admin"}}})
	require.ErrorContains(t, CheckExternalDeployment(), "identity_url")

	viper.Set("auth.scim.enabled", false)
	require.NoError(t, CheckExternalDeployment())

	viper.Set("auth.scim.enabled", true)
	viper.Set("auth.scim.identity_url", "https://idp.example")
	require.NoError(t, CheckExternalDeployment())
}
