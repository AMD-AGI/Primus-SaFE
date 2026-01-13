// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import "errors"

// Authentication errors
var (
	ErrInvalidCredentials  = errors.New("invalid username or password")
	ErrUserDisabled        = errors.New("user account is disabled")
	ErrUserLocked          = errors.New("user account is locked")
	ErrUserNotFound        = errors.New("user not found")
	ErrLDAPNotConfigured   = errors.New("LDAP authentication is not configured")
	ErrAuthModeNotSupported = errors.New("authentication mode not supported")
	ErrSessionExpired      = errors.New("session has expired")
	ErrSessionRevoked      = errors.New("session has been revoked")
)
