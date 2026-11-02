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
	// offers registration or password changes, and creates API keys only where the deployment routes them
	// to the provider, see APIKeysURL).
	AuthMode string `json:"authMode"`
	// With external authentication, the path on the console's own origin where API key creation, listing and
	// revocation are routed to the identity provider (auth.api_keys_url). Absent when authentication is not
	// external or nothing is routed there: the console then offers no key creation.
	APIKeysURL string `json:"apiKeysUrl,omitempty"`
	// Whether to enable sso
	SSOEnable bool `json:"ssoEnable"`
	// The url for sso authorization
	SSOAuthUrl string `json:"ssoAuthUrl"`
	// Origins (optionally with a path prefix) other than the web console itself that may receive an SSO code.
	SSOBridgeAllowlist []string `json:"ssoBridgeAllowlist"`
	// Whether CD deployment requires approval from another user
	CDRequireApproval bool `json:"cdRequireApproval"`
	// KubeSpray image tags mapped to the Kubernetes versions they install.
	KubeSprayK8sVersions map[string]string `json:"kubeSprayK8sVersions"`
}
