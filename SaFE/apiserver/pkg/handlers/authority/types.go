/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

const (
	CookieToken = "Token"

	UserWorkspaceResource = "user/workspace"
	UserIdentityResource  = "user/identity"
	SecretResourceKind    = "secret"
	PreflightKind         = "preflight"
	PublicKeyKind         = "PublicKey"
	ImageImportKind       = "ImageImport"
	ImageRegisterKind     = "ImageRegister"
	AllResource           = "*"

	GrantedAllUser       = "*"
	GrantedOwner         = "owner"
	GrantedWorkspaceUser = "workspace-user"
)
