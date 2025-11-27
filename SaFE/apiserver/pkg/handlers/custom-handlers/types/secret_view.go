/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type SecretParam string

const (
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
	// Used to generate the secret ID, which will do normalization processing, e.g. lowercase
	Name string `json:"name"`
	// The workspaces which the secret belongs to
	WorkspaceIds []string `json:"workspaceIds,omitempty"`
	// Secret type, e.g. image, ssh, default
	Type v1.SecretType `json:"type"`
	// Parameters required for creating the secret, including username, password, privateKey, publicKey and so on
	// for a general secret, you can define any parameters.
	// the private key, public key and password need to be Base64 encoded.
	// each server can have only one auth entry.
	// Multiple auths may be created for image secret, so the params is a slice
	Params []map[SecretParam]string `json:"params"`
	// The secret owner, For internal use.
	Owner string `json:"-"`
}

type CreateSecretResponse struct {
	// Secret ID
	SecretId string `json:"secretId"`
}

type ListSecretRequest struct {
	// Secret type, e.g. ssh, image, general
	// if specifying multiple phase queries, separate them with commas
	Type string `form:"type" binding:"omitempty"`
	// the workspace which the secret belongs to
	WorkspaceId *string `json:"workspaceId,omitempty"`
}

type ListSecretResponse struct {
	// The total number of secrets, not limited by pagination
	TotalCount int                  `json:"totalCount"`
	Items      []SecretResponseItem `json:"items,omitempty"`
}

type SecretResponseItem struct {
	// Secret ID
	SecretId string `json:"secretId"`
	// Secret name
	SecretName string `json:"secretName"`
	// The workspaces which the secret belongs to
	WorkspaceIds []string `json:"workspaceIds"`
	// Secret type, e.g. ssh, image
	Type string `json:"type"`
	// Creation timestamp of the secret
	CreationTime string `json:"creationTime"`
	// The userId who created the secret
	UserId string `json:"userId"`
	// The userName who created the secret
	UserName string `json:"userName"`
}

type GetSecretResponse struct {
	SecretResponseItem
	// Parameters required for creating the secret, including username, password, privateKey, publicKey.
	Params []map[SecretParam]string `json:"params"`
}

type DockerConfigItem struct {
	UserName string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

type DockerConfig struct {
	Auths map[string]DockerConfigItem `json:"auths"`
}

type PatchSecretRequest struct {
	// Parameters required for creating the secret, including username, password, privateKey, publicKey.
	// the private key, public key and password need to be Base64 encoded.
	// each server can have only one auth entry.
	// Multiple auths may be created for image secret, so the params is a slice
	// When provided, the params list will REPLACE the existing parameters.
	Params *[]map[SecretParam]string `json:"params,omitempty"`
	// the workspaces which the secret belongs to.
	// When provided, this list will REPLACE the existing bound workspaces.
	WorkspaceIds *[]string `json:"workspaceIds,omitempty"`
}
