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
	Name string `json:"name,omitempty"`
	// secret type. crypto/image/ssh
	Type SecretType `json:"type"`
	// Parameters required for creating the secret, including username, password, privateKey, publicKey.
	// Both the private key and public key need to be Base64 encoded
	Params map[SecretParam]string `json:"params"`
}

type CreateSecretResponse struct {
	SecretId string `json:"secretId"`
}

type ListSecretRequest struct {
	Type string `form:"type" binding:"omitempty"`
}

type ListSecretResponse struct {
	TotalCount int                  `json:"totalCount"`
	Items      []SecretResponseItem `json:"items,omitempty"`
}

type SecretResponseItem struct {
	SecretId   string `json:"secretId"`
	SecretName string `json:"secretName"`
	Type       string `json:"type,omitempty"`
	UserName   string `json:"userName,omitempty"`
	// Creation timestamp of the secret
	CreationTime string `json:"creationTime"`
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
