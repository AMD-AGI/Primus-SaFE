/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

const (
	CookieToken    = "Token"
	CookieUserType = "userType"

	UserWorkspaceResource  = "user/workspace"
	UserIdentityResource   = "user/identity"
	SecretResourceKind     = "secret"
	PreflightKind          = "opsjob/preflight"
	DownloadKind           = "opsjob/download"
	DumpLogKind            = "opsjob/dumplog"
	PublicKeyKind          = "PublicKey"
	ImageImportKind        = "ImageImport"
	ImageRegisterKind      = "ImageRegister"
	ApiKeysKind            = "apikeys"
	WorkloadPrivilegedKind = "workload/privileged"
	AllResource            = "*"

	GrantedAllUser       = "*"
	GrantedOwner         = "owner"
	GrantedWorkspaceUser = "workspace-user"
)
