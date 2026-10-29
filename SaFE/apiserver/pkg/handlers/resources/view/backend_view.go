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
	// How requests are authenticated: "builtin" (accounts, sessions and API keys of this
	// deployment) or "external" (an identity provider decides, and the console neither
	// offers registration, password changes nor API key creation).
	AuthMode string `json:"authMode"`
	// Whether to enable sso
	SSOEnable bool `json:"ssoEnable"`
	// The url for sso authorization
	SSOAuthUrl string `json:"ssoAuthUrl"`
	// Whether CD deployment requires approval from another user
	CDRequireApproval bool `json:"cdRequireApproval"`
	// KubeSpray image tags mapped to the Kubernetes versions they install.
	KubeSprayK8sVersions map[string]string `json:"kubeSprayK8sVersions"`
}
