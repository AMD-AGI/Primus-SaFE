/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// Console URLs keep a path or an http(s) URL and drop anything else; the accounts and API keys pages default to the identity console. The provider client and OIDC-only switches default off.
func TestConsoleURLAndProviderSwitches(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.False(t, AllowOIDCOnly())
	require.False(t, ProviderClientEnabled())
	require.Equal(t, DefaultAuthConnection, GetAuthConnection())
	require.Equal(t, DefaultAccountsURL, GetAuthAccountsURL())
	require.Equal(t, DefaultKeysConsoleURL, GetAuthKeysConsoleURL())
	require.Equal(t, DefaultSCIMTokenSecret, GetSCIMTokenSecret())

	viper.Set("auth.accounts_url", "  ")
	viper.Set("auth.keys_console_url", "")
	require.Equal(t, "/identity/#accounts", GetAuthAccountsURL())
	require.Equal(t, "/identity/#keys", GetAuthKeysConsoleURL())

	viper.Set("auth.allow_oidc_only", true)
	viper.Set("auth.provider_client", true)
	viper.Set("auth.connection", "  local  ")
	viper.Set("auth.accounts_url", "/idp/accounts")
	viper.Set("auth.keys_console_url", "https://idp.example/keys")
	viper.Set("auth.scim.token_secret", "  ")
	require.True(t, AllowOIDCOnly())
	require.True(t, ProviderClientEnabled())
	require.Equal(t, "local", GetAuthConnection())
	require.Equal(t, "/idp/accounts", GetAuthAccountsURL())
	require.Equal(t, "https://idp.example/keys", GetAuthKeysConsoleURL())
	require.Equal(t, DefaultSCIMTokenSecret, GetSCIMTokenSecret())

	require.Empty(t, consoleURL("ftp://idp.example/keys"))
	require.Empty(t, consoleURL("//idp.example"))
	require.Empty(t, consoleURL("not a url"))
}
