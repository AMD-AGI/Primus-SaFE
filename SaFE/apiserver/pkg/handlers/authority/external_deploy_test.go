/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// External mode needs a login URL and a group that grants system-admin. The identity service URL is not
// needed to start: a SCIM registration without it logs its failure.
func TestCheckExternalDeployment(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	require.NoError(t, CheckExternalDeployment())

	viper.Set("auth.mode", "external")
	require.ErrorContains(t, CheckExternalDeployment(), "login_url")

	viper.Set("auth.login_url", "https://idp.example/sign-in")
	require.ErrorContains(t, CheckExternalDeployment(), "system-admin")

	viper.Set("auth.group_mappings", []map[string]any{{"group": "readers", "roles": []string{"system-admin-readonly"}}})
	require.ErrorContains(t, CheckExternalDeployment(), "system-admin to no group")

	viper.Set("auth.group_mappings", []map[string]any{{"group": "admins", "roles": []string{"system-admin"}}})
	require.NoError(t, CheckExternalDeployment(), "SCIM on without an identity URL does not block start-up")

	viper.Set("auth.scim.enabled", false)
	require.NoError(t, CheckExternalDeployment())

	viper.Set("auth.scim.enabled", true)
	viper.Set("auth.scim.identity_url", "https://idp.example")
	require.NoError(t, CheckExternalDeployment())

	viper.Set("auth.scim.identity_url", "")
	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection", "url": "https://idp.example/oauth/introspect"}})
	require.NoError(t, CheckExternalDeployment(), "the introspection URL is the identity service URL")
}

// The console's ID token (audience sso.login.client_id) is never a credential, so an external chain with
// sso.login on refuses an oidc authenticator for that audience; any other entry is left alone.
func TestRefuseLoginAudience(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	oidc := func(aud string) commonconfig.AuthenticatorConfig {
		return commonconfig.AuthenticatorConfig{Type: "oidc", Audience: aud}
	}
	require.NoError(t, refuseLoginAudience([]commonconfig.AuthenticatorConfig{oidc("safe-console")}), "sso.login off")

	viper.Set("sso.login.enable", true)
	require.ErrorContains(t, refuseLoginAudience([]commonconfig.AuthenticatorConfig{oidc("primus-safe")}), "client_id is empty")

	viper.Set("sso.login.client_id", "safe-console")
	require.ErrorContains(t, refuseLoginAudience([]commonconfig.AuthenticatorConfig{oidc("primus-safe"), oidc("safe-console")}),
		`auth.authenticators[1] is an oidc entry for audience "safe-console"`)
	require.ErrorContains(t, refuseLoginAudience([]commonconfig.AuthenticatorConfig{{Type: "OIDC", Audience: "safe-console"}}), "must not be a credential")
	require.NoError(t, refuseLoginAudience([]commonconfig.AuthenticatorConfig{{Type: "introspection", Audience: "safe-console"}}),
		"an introspection entry is asked about opaque credentials, not the ID token")
	require.NoError(t, refuseLoginAudience([]commonconfig.AuthenticatorConfig{oidc("primus-safe"), {Type: "introspection", Audience: "primus-safe"}}))
	require.NoError(t, refuseLoginAudience(nil))
}

// buildChain refuses an empty chain, an oidc-only chain unless auth.allow_oidc_only is set, a malformed
// duration, and an unknown type.
func TestBuildChainRefusals(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	oidc := commonconfig.AuthenticatorConfig{Type: "oidc", Issuer: "https://idp.example", Audience: "primus-safe", JWKSURL: "https://idp.example/jwks"}

	_, err := buildChain(nil)
	require.ErrorContains(t, err, "auth.authenticators is empty")

	_, err = buildChain([]commonconfig.AuthenticatorConfig{oidc})
	require.ErrorContains(t, err, "allow_oidc_only")

	_, err = buildChain([]commonconfig.AuthenticatorConfig{{Type: "kerberos"}})
	require.ErrorContains(t, err, "allow_oidc_only")

	_, err = buildChain([]commonconfig.AuthenticatorConfig{{Type: "kerberos"}, {Type: "builtin"}})
	require.ErrorContains(t, err, `unknown type "kerberos"`)

	bad := oidc
	bad.ClockSkew = "soon"
	_, err = buildChain([]commonconfig.AuthenticatorConfig{bad, {Type: "builtin"}})
	require.ErrorContains(t, err, "clock_skew")

	chain, err := buildChain([]commonconfig.AuthenticatorConfig{{Type: "builtin"}})
	require.NoError(t, err)
	require.Equal(t, 1, chain.Len())

	viper.Set("auth.allow_oidc_only", true)
	chain, err = buildChain([]commonconfig.AuthenticatorConfig{oidc})
	require.NoError(t, err)
	require.Equal(t, 1, chain.Len())
}
