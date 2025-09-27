/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

type SecretType string
type SecretParam string

const (
	SecretImage SecretType = "image"
	SecretSSH   SecretType = "ssh"

	PasswordParam   SecretParam = "password"
	PrivateKeyParam SecretParam = "privateKey"
	PublicKeyParam  SecretParam = "publicKey"
	UserNameParam   SecretParam = "username"
	ServerParam     SecretParam = "server"

	DockerConfigJson = ".dockerconfigjson"
	SSHAuthKey       = "authorize"
	SSHAuthPubKey    = "authorize.pub"
)

type CreateSecretRequest struct {
	// Secret name (display only), applicable only for SSH key usage
	Name string `json:"name,omitempty"`
	// secret type. crypto/image/ssh
	Type SecretType `json:"type"`
	// Parameters required for creating the secret, including username, password, privateKey, publicKey.
	// the private key, public key and password need to be Base64 encoded
	Params map[SecretParam]string `json:"params"`
}

type CreateSecretResponse struct {
	SecretId string `json:"secretId"`
}

type ListSecretRequest struct {
	// secret type: ssh/image
	// if specifying multiple phase queries, separate them with commas
	Type string `form:"type" binding:"omitempty"`
}

type ListSecretResponse struct {
	TotalCount int                  `json:"totalCount"`
	Items      []SecretResponseItem `json:"items,omitempty"`
}

type SecretResponseItem struct {
	SecretId   string `json:"secretId"`
	SecretName string `json:"secretName"`
	Type       string `json:"type"`
	// Parameters required for creating the secret, including username, password, privateKey, publicKey.
	Params map[SecretParam]string `json:"params"`
	// Creation timestamp of the secret
	CreationTime string `json:"creationTime"`
}

type DockerConfigItem struct {
	UserName string `json:"username"`
	Password string `json:"password"`
}

type DockerConfig struct {
	Auth map[string]DockerConfigItem `json:"auths"`
}

type PatchSecretRequest struct {
	// Parameters required for creating the secret, including username, password, privateKey, publicKey.
	// the private key, public key and password need to be Base64 encoded
	Params map[SecretParam]string `json:"params,omitempty"`
}
