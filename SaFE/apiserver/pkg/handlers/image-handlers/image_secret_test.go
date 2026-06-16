/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

// TestBuildImagePullSecrets verifies secret names are wrapped into object references.
func TestBuildImagePullSecrets(t *testing.T) {
	refs := buildImagePullSecrets([]string{"a", "b"})
	assert.Len(t, refs, 2)
	assert.Equal(t, "a", refs[0].Name)
	assert.Equal(t, "b", refs[1].Name)

	assert.Len(t, buildImagePullSecrets(nil), 0)
}

// TestExtractAuthFromSecretDirect verifies username/password is read directly.
func TestExtractAuthFromSecretDirect(t *testing.T) {
	h := &ImageHandler{}
	secret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"harbor.example.com":{"username":"u","password":"p"}}}`),
		},
	}
	auth, err := h.extractAuthFromSecret("harbor.example.com", secret)
	assert.NoError(t, err)
	assert.NotNil(t, auth)
	assert.Equal(t, "u", auth.Username)
	assert.Equal(t, "p", auth.Password)
}

// TestExtractAuthFromSecretBase64 verifies credentials are decoded from the auth field.
func TestExtractAuthFromSecretBase64(t *testing.T) {
	h := &ImageHandler{}
	encoded := base64.StdEncoding.EncodeToString([]byte("user1:pass1"))
	secret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"reg.io":{"auth":"` + encoded + `"}}}`),
		},
	}
	auth, err := h.extractAuthFromSecret("reg.io", secret)
	assert.NoError(t, err)
	assert.NotNil(t, auth)
	assert.Equal(t, "user1", auth.Username)
	assert.Equal(t, "pass1", auth.Password)
}

// TestExtractAuthFromSecretNoConfig verifies a missing dockerconfigjson returns nil.
func TestExtractAuthFromSecretNoConfig(t *testing.T) {
	h := &ImageHandler{}
	secret := &corev1.Secret{Data: map[string][]byte{}}
	auth, err := h.extractAuthFromSecret("reg.io", secret)
	assert.NoError(t, err)
	assert.Nil(t, auth)
}

// TestExtractAuthFromSecretNoMatch verifies a non-matching host returns nil.
func TestExtractAuthFromSecretNoMatch(t *testing.T) {
	h := &ImageHandler{}
	secret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"other.io":{"username":"u","password":"p"}}}`),
		},
	}
	auth, err := h.extractAuthFromSecret("reg.io", secret)
	assert.NoError(t, err)
	assert.Nil(t, auth)
}

// TestExtractAuthFromSecretBadJSON verifies invalid config json reports an error.
func TestExtractAuthFromSecretBadJSON(t *testing.T) {
	h := &ImageHandler{}
	secret := &corev1.Secret{
		Data: map[string][]byte{".dockerconfigjson": []byte("not-json")},
	}
	_, err := h.extractAuthFromSecret("reg.io", secret)
	assert.Error(t, err)
}
