// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

// AuthMode represents the authentication mode
type AuthMode string

const (
	// AuthModeNone means no authentication required (dev/test mode)
	AuthModeNone AuthMode = "none"
	// AuthModeLocal means local database authentication
	AuthModeLocal AuthMode = "local"
	// AuthModeLDAP means LDAP/AD authentication
	AuthModeLDAP AuthMode = "ldap"
	// AuthModeSSO means OIDC SSO authentication
	AuthModeSSO AuthMode = "sso"
	// AuthModeSaFE means Primus-SaFE integration mode
	AuthModeSaFE AuthMode = "safe"
)

// AuthType represents the authentication source type
type AuthType string

const (
	AuthTypeLocal AuthType = "local"
	AuthTypeLDAP  AuthType = "ldap"
	AuthTypeSafe  AuthType = "safe"
)

// UserStatus represents the user account status
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
	UserStatusLocked   UserStatus = "locked"
)

// SyncSource represents the source of a synced session
type SyncSource string

const (
	SyncSourceLocal SyncSource = "local"
	SyncSourceSafe  SyncSource = "safe"
)

// System config keys
const (
	ConfigKeyAuthMode                    = "auth.mode"
	ConfigKeyAuthInitialized             = "auth.initialized"
	ConfigKeySystemInitialized           = "system.initialized"
	ConfigKeySafeIntegrationEnabled      = "safe.integration.enabled"
	ConfigKeySafeIntegrationAutoDetected = "safe.integration.auto_detected"
	ConfigKeySafeAdapterURL              = "safe.adapter_url"
	ConfigKeySafeSSOURL                  = "safe.sso_url"
)

// Root user constants
const (
	RootUserID      = "root"
	RootUsername    = "root"
	RootEmail       = "root@localhost"
	RootDisplayName = "Root Administrator"
)

// Environment variable keys
const (
	EnvRootPassword = "LENS_ROOT_PASSWORD"
)
