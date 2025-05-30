/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

type SecretType string
type SecretParam string

const (
	SecretCrypto SecretType = "crypto"
	SecretImage  SecretType = "image"
	SecretSSH    SecretType = "ssh"

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
	DisplayName string `json:"displayName,omitempty"`
	// secret type. crypto/image/ssh
	Type SecretType `json:"type"`
	// Parameters required for creating the secret, including username, password, privateKey, publicKey.
	// Both the private key and public key need to be Base64 encoded
	Params map[SecretParam]string `json:"params"`
}

type CreateSecretResponse struct {
	SecretId string `json:"secretId"`
}

type GetSecretRequest struct {
	Type string `form:"type" binding:"omitempty"`
}

type GetSecretResponse struct {
	TotalCount int                     `json:"totalCount"`
	Items      []GetSecretResponseItem `json:"items,omitempty"`
}

type GetSecretResponseItem struct {
	SecretId   string `json:"secretId"`
	SecretName string `json:"secretName"`
	Type       string `json:"type,omitempty"`
}

type DockerConfigItem struct {
	UserName string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

type DockerConfig struct {
	Auth map[string]DockerConfigItem `json:"auths"`
}

func (req *CreateSecretRequest) HasParam(key SecretParam) bool {
	val, _ := req.Params[key]
	return val != ""
}
