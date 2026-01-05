/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

type GetEnvResponse struct {
	// Whether to enable log download.
	EnableLogDownload bool `json:"enableLogDownload"`
	// Whether to enable the entire logging functionality, including log download.
	EnableLog bool `json:"enableLog"`
	// Whether to enable ssh include webshell.
	EnableSSH bool `json:"enableSsh"`
	// The port for ssh
	SSHPort int `json:"sshPort"`
	// The ip for ssh
	SSHIP string `json:"sshIP"`
	// Whether to enable sso
	SSOEnable bool `json:"ssoEnable"`
	// The url for sso authorization
	SSOAuthUrl string `json:"ssoAuthUrl"`
	// Whether CD deployment requires approval from another user
	CDRequireApproval bool `json:"cdRequireApproval"`
}
