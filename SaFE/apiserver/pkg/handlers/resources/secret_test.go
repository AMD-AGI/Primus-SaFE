/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// TestCvtToSecretResponseItem tests conversion from corev1.Secret to SecretResponseItem
func TestCvtToSecretResponseItem(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		secret   *corev1.Secret
		validate func(*testing.T, view.GetSecretResponse)
	}{
		{
			name: "SSH secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "ssh-secret-test",
					CreationTimestamp: metav1.NewTime(now),
					Labels: map[string]string{
						v1.DisplayNameLabel: "Test SSH Secret",
						v1.SecretTypeLabel:  string(v1.SecretSSH),
					},
				},
				Data: map[string][]byte{
					string(view.UserNameParam): []byte("testuser"),
					view.SSHAuthKey:            []byte("private-key-content"),
					view.SSHAuthPubKey:         []byte("public-key-content"),
				},
			},
			validate: func(t *testing.T, result view.GetSecretResponse) {
				assert.Equal(t, "ssh-secret-test", result.SecretId)
				assert.Equal(t, "Test SSH Secret", result.SecretName)
				assert.Equal(t, string(v1.SecretSSH), result.Type)
				assert.Len(t, result.Params, 1)

				params := result.Params[0]
				assert.Equal(t, "testuser", params[view.UserNameParam])
				assert.Equal(t, stringutil.Base64Encode("private-key-content"), params[view.PrivateKeyParam])
				assert.Equal(t, stringutil.Base64Encode("public-key-content"), params[view.PublicKeyParam])
			},
		},
		{
			name: "Image registry secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "registry-secret",
					CreationTimestamp: metav1.NewTime(now),
					Labels: map[string]string{
						v1.DisplayNameLabel: "Docker Registry",
						v1.SecretTypeLabel:  string(v1.SecretImage),
					},
				},
				Data: map[string][]byte{
					view.DockerConfigJson: genDockerConfigData(t, "docker.io", "username", "password"),
				},
			},
			validate: func(t *testing.T, result view.GetSecretResponse) {
				assert.Equal(t, "registry-secret", result.SecretId)
				assert.Equal(t, "Docker Registry", result.SecretName)
				assert.Equal(t, string(v1.SecretImage), result.Type)
				assert.Len(t, result.Params, 1)

				params := result.Params[0]
				assert.Equal(t, "docker.io", params[view.ServerParam])
				assert.Equal(t, "username", params[view.UserNameParam])
				assert.Equal(t, stringutil.Base64Encode("password"), params[view.PasswordParam])
			},
		},
		{
			name: "Multi-registry secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "multi-registry",
					CreationTimestamp: metav1.NewTime(now),
					Labels: map[string]string{
						v1.SecretTypeLabel: string(v1.SecretImage),
					},
				},
				Data: map[string][]byte{
					view.DockerConfigJson: genMultiDockerConfigData(t, map[string]view.DockerConfigItem{
						"docker.io": {UserName: "user1", Password: "pass1"},
						"gcr.io":    {UserName: "user2", Password: "pass2"},
					}),
				},
			},
			validate: func(t *testing.T, result view.GetSecretResponse) {
				assert.Equal(t, "multi-registry", result.SecretId)
				assert.Equal(t, string(v1.SecretImage), result.Type)
				assert.Len(t, result.Params, 2)

				// Check both registries are present
				servers := make([]string, len(result.Params))
				for i, params := range result.Params {
					servers[i] = params[view.ServerParam]
				}
				assert.Contains(t, servers, "docker.io")
				assert.Contains(t, servers, "gcr.io")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToGetSecretResponse(tt.secret)
			tt.validate(t, result)
			// Verify creation time is formatted
			assert.Contains(t, result.CreationTime, now.Format("2006-01-02"))
		})
	}
}

// TestBuildSecretLabelSelector tests label selector construction for secrets
func TestBuildSecretLabelSelector(t *testing.T) {
	tests := []struct {
		name     string
		query    *view.ListSecretRequest
		validate func(*testing.T, string)
	}{
		{
			name: "filter by single type",
			query: &view.ListSecretRequest{
				Type: "ssh",
			},
			validate: func(t *testing.T, selector string) {
				assert.Contains(t, selector, v1.SecretTypeLabel)
				assert.Contains(t, selector, "ssh")
			},
		},
		{
			name: "filter by multiple types",
			query: &view.ListSecretRequest{
				Type: "ssh,image",
			},
			validate: func(t *testing.T, selector string) {
				assert.Contains(t, selector, v1.SecretTypeLabel)
				assert.Contains(t, selector, "in")
			},
		},
		{
			name: "no filter",
			query: &view.ListSecretRequest{
				Type: "",
			},
			validate: func(t *testing.T, selector string) {
				// Should return empty selector
				assert.Empty(t, selector)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := buildSecretLabelSelector(tt.query)
			tt.validate(t, selector.String())
		})
	}
}

// Helper function to generate Docker config JSON data
func genDockerConfigData(t *testing.T, server, username, password string) []byte {
	config := view.DockerConfig{
		Auths: map[string]view.DockerConfigItem{
			server: {
				UserName: username,
				Password: password,
			},
		},
	}
	data, err := json.Marshal(config)
	assert.NoError(t, err)
	return data
}

// Helper function to generate multi-registry Docker config
func genMultiDockerConfigData(t *testing.T, auths map[string]view.DockerConfigItem) []byte {
	config := view.DockerConfig{
		Auths: auths,
	}
	data, err := json.Marshal(config)
	assert.NoError(t, err)
	return data
}
