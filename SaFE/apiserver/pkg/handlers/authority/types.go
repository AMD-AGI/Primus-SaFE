/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

const (
	CookieToken    = "Token"
	CookieUserType = "UserType"

	UserWorkspaceResource = "user/workspace"
	UserIdentityResource  = "user/identity"
	SecretResourceKind    = "secret"
	PreflightKind         = "opsjob/preflight"
	PublicKeyKind         = "PublicKey"
	ImageImportKind       = "ImageImport"
	ImageRegisterKind     = "ImageRegister"
	AllResource           = "*"

	GrantedAllUser       = "*"
	GrantedOwner         = "owner"
	GrantedWorkspaceUser = "workspace-user"
)
