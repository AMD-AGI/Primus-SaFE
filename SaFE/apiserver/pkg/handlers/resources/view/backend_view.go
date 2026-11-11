/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

type GetEnvResponse struct {
	// Whether a Robust Addon is installed. The console queries GPU aggregation only when this is true.
	EnableRobust bool `json:"enableRobust"`
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
	// Where the console sends the browser to sign in when AuthMode is external (auth.login_url).
	// Absent unless authentication is external and the deployment sets one. Not an SSO URL.
	LoginURL string `json:"loginUrl,omitempty"`
	// The one identity connection the console names on the authorize request when AuthMode is external.
	// okta-amd or local. Absent when authentication is not external.
	Connection string `json:"connection,omitempty"`
	// Whether to enable sso. This is sso.enable. The console uses it only when AuthMode is not external.
	SSOEnable bool `json:"ssoEnable"`
	// The authorization URL of the sso Secret provider, when AuthMode is not external and that provider
	// is configured. The console uses it when SSOLoginUrl is empty. Absent in external mode.
	SSOAuthUrl string `json:"ssoAuthUrl,omitempty"`
	// The console's sign-in at the sso.login provider, when AuthMode is not external and sso.login is on:
	// an authorization URL that needs PKCE (the console adds its state and an S256 code_challenge, and
	// sends the code verifier with the code).
	SSOLoginUrl string `json:"ssoLoginUrl,omitempty"`
	// True when sso.login is on, even while its provider cannot be reached (ssoLoginUrl empty): the
	// console then signs in only with PKCE.
	SSOPkce bool `json:"ssoPkce,omitempty"`
	// Origins (optionally with a path prefix) other than the web console itself that may receive an SSO code.
	SSOBridgeAllowlist []string `json:"ssoBridgeAllowlist"`
	// Whether CD deployment requires approval from another user
	CDRequireApproval bool `json:"cdRequireApproval"`
	// The line the console's "no access yet" page shows to a user who holds no access (console.access_contact).
	// Plain text, empty when the deployment sets none.
	AccessContact string `json:"accessContact"`
	// KubeSpray image tags mapped to the Kubernetes versions they install.
	KubeSprayK8sVersions map[string]string `json:"kubeSprayK8sVersions"`
}
