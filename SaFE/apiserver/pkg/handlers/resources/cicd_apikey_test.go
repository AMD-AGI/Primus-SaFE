/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	safecrypto "github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
)

// cicdTestContext returns a gin context for POST /api/v1/workloads with the given Authorization header.
func cicdTestContext(authorization string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)
	if authorization != "" {
		c.Request.Header.Set("Authorization", authorization)
	}
	return c
}

// cicdTestWorkload returns a workload of the given kind owned by ownerId.
func cicdTestWorkload(name, kind, ownerId string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: ownerId}}}
	w.Spec.GroupVersionKind.Kind = kind
	return w
}

// stubCICDPlatformKey replaces the database platform key lookup for one test.
func stubCICDPlatformKey(t *testing.T, key string) *[]string {
	t.Helper()
	var seen []string
	orig := cicdPlatformKeyForUser
	cicdPlatformKeyForUser = func(_ context.Context, userId, _ string) (string, error) {
		seen = append(seen, userId)
		return key, nil
	}
	t.Cleanup(func() { cicdPlatformKeyForUser = orig })
	return &seen
}

// setAuthMode sets auth.mode for one test.
func setAuthMode(t *testing.T, mode string) {
	t.Helper()
	prev := viper.Get("auth.mode")
	viper.Set("auth.mode", mode)
	t.Cleanup(func() { viper.Set("auth.mode", prev) })
}

// cicdTestUser returns a user with the given name (user id).
func cicdTestUser(name string) *v1.User { return &v1.User{ObjectMeta: metav1.ObjectMeta{Name: name}} }

func TestResolveCICDAPIKeyPlainBuiltinUsesOwnerPlatformKey(t *testing.T) {
	setAuthMode(t, "builtin")
	seen := stubCICDPlatformKey(t, "ak-platform-u1")
	c := cicdTestContext("Bearer ak-user-created")

	got, err := resolveCICDAPIKeyPlain(c, cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u1"), cicdTestUser("u1"))
	require.NoError(t, err)
	assert.Equal(t, "ak-platform-u1", got)
	assert.Equal(t, []string{"u1"}, *seen)
}

func TestResolveCICDAPIKeyPlainRefusesNonOwner(t *testing.T) {
	for _, mode := range []string{"builtin", "external"} {
		t.Run(mode, func(t *testing.T) {
			setAuthMode(t, mode)
			seen := stubCICDPlatformKey(t, "ak-platform-owner")
			c := cicdTestContext("Bearer ak-admin")

			_, err := resolveCICDAPIKeyPlain(c, cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "owner"), cicdTestUser("admin"))
			require.Error(t, err)
			assert.True(t, apierrors.IsForbidden(err))
			assert.Empty(t, *seen)
		})
	}
}

// fakeIdentityPlatform serves the platform key endpoint for one test and records the last request.
func fakeIdentityPlatform(t *testing.T) *http.Request {
	t.Helper()
	got := &http.Request{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = *r.Clone(context.Background())
		_, _ = w.Write([]byte(`{"userId":"u1","apiKey":"ak-platform-u1"}`))
	}))
	t.Cleanup(srv.Close)
	secret := filepath.Join(t.TempDir(), "client_secret")
	require.NoError(t, os.WriteFile(secret, []byte("secret\n"), 0o600))
	viper.Set("auth.scim.identity_url", srv.URL)
	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection", "client_id": "svc", "client_secret_file": secret}})
	t.Cleanup(func() {
		viper.Set("auth.scim.identity_url", nil)
		viper.Set("auth.authenticators", nil)
	})
	return got
}

func TestResolveCICDAPIKeyPlainExternalAPIKeyFetchesPlatformKey(t *testing.T) {
	setAuthMode(t, "external")
	got := fakeIdentityPlatform(t)
	c := cicdTestContext("Bearer ak-user-key")
	c.Set(common.UserType, "external")

	key, err := resolveCICDAPIKeyPlain(c, cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u1"), cicdTestUser("u1"))
	require.NoError(t, err)
	assert.Equal(t, "ak-platform-u1", key)
	assert.Equal(t, "/api/v1/apikeys/platform", got.URL.Path)
	assert.Equal(t, "Bearer ak-user-key", got.Header.Get("Authorization"))
	assert.Equal(t, "svc:secret", got.Header.Get("X-Internal-Token"))
}

// A key the built-in mode at the end of the external chain accepted is SaFE's own: the owner's platform key comes
// from SaFE's database, and identity is not asked.
func TestResolveCICDAPIKeyPlainExternalBuiltinUsesOwnerPlatformKey(t *testing.T) {
	setAuthMode(t, "external")
	got := fakeIdentityPlatform(t)
	seen := stubCICDPlatformKey(t, "ak-db-u1")
	c := cicdTestContext("Bearer ak-safe-issued")
	c.Set(common.UserType, "apikey")

	key, err := resolveCICDAPIKeyPlain(c, cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u1"), cicdTestUser("u1"))
	require.NoError(t, err)
	assert.Equal(t, "ak-db-u1", key)
	assert.Equal(t, []string{"u1"}, *seen)
	assert.Empty(t, got.URL, "identity was asked")
}

// The bearer the external chain authenticated is the credential sent to identity, not a cookie of another session.
func TestResolveCICDAPIKeyPlainExternalPrefersBearerOverCookie(t *testing.T) {
	setAuthMode(t, "external")
	got := fakeIdentityPlatform(t)
	c := cicdTestContext("Bearer sso-token-u1")
	c.Request.AddCookie(&http.Cookie{Name: "Token", Value: "stale-cookie-u2"})
	c.Request.Header.Set("UserType", "sso")
	c.Set(common.UserType, "external")

	_, err := resolveCICDAPIKeyPlain(c, cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u1"), cicdTestUser("u1"))
	require.NoError(t, err)
	assert.Equal(t, "Bearer sso-token-u1", got.Header.Get("Authorization"))
	assert.Empty(t, got.Header.Get("Cookie"))
	assert.Equal(t, "sso", got.Header.Get("UserType"))
}

// A platform key identity answers for another user is refused.
func TestResolveCICDAPIKeyPlainExternalRefusesAnotherUsersKey(t *testing.T) {
	setAuthMode(t, "external")
	fakeIdentityPlatform(t)
	c := cicdTestContext("Bearer sso-token")
	c.Set(common.UserType, "external")

	_, err := resolveCICDAPIKeyPlain(c, cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u2"), cicdTestUser("u2"))
	require.Error(t, err)
}

// An unavailable crypto client is an internal error, not a panic.
func TestAttachCICDEncryptedAPIKeyWithoutCrypto(t *testing.T) {
	setAuthMode(t, "builtin")
	stubCICDPlatformKey(t, "ak-cicd-key")
	orig := cicdCrypto
	cicdCrypto = func() *safecrypto.Crypto { return nil }
	t.Cleanup(func() { cicdCrypto = orig })

	workload := cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u1")
	err := (&Handler{}).attachCICDEncryptedAPIKey(cicdTestContext(""), workload, cicdTestUser("u1"))
	require.Error(t, err)
	assert.True(t, apierrors.IsInternalError(err))
	assert.Empty(t, v1.GetAnnotation(workload, v1.UserEncryptedAPIKeyAnnotation))
}

func TestResolveCICDAPIKeyPlainExternalSSOTokenFetchesPlatformKey(t *testing.T) {
	setAuthMode(t, "external")
	got := fakeIdentityPlatform(t)

	c := cicdTestContext("Bearer sso-access-token")
	c.Request.Header.Set("UserType", "sso")
	c.Set(common.UserType, "external")

	key, err := resolveCICDAPIKeyPlain(c, cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u1"), cicdTestUser("u1"))
	require.NoError(t, err)
	assert.Equal(t, "ak-platform-u1", key)
	assert.Equal(t, "/api/v1/apikeys/platform", got.URL.Path)
	assert.Equal(t, "Bearer sso-access-token", got.Header.Get("Authorization"))
	assert.Equal(t, "sso", got.Header.Get("UserType"))
}

func TestResolveCICDAPIKeyPlainExternalNeedsSession(t *testing.T) {
	setAuthMode(t, "external")
	c := cicdTestContext("")
	c.Set(common.UserType, "external")

	_, err := resolveCICDAPIKeyPlain(c, cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u1"), cicdTestUser("u1"))
	require.Error(t, err)
	assert.True(t, apierrors.IsUnauthorized(err))
}

func TestAttachCICDEncryptedAPIKey(t *testing.T) {
	commonconfig.SetValue("crypto.enable", "false")
	t.Cleanup(func() { commonconfig.SetValue("crypto.enable", "") })
	setAuthMode(t, "builtin")
	stubCICDPlatformKey(t, "ak-cicd-key")
	c := cicdTestContext("")
	h := &Handler{}

	workload := cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u1")
	require.NoError(t, h.attachCICDEncryptedAPIKey(c, workload, cicdTestUser("u1")))
	assert.Equal(t, "ak-cicd-key", v1.GetAnnotation(workload, v1.UserEncryptedAPIKeyAnnotation))

	other := cicdTestWorkload("train-1", common.PytorchJobKind, "u1")
	require.NoError(t, h.attachCICDEncryptedAPIKey(c, other, cicdTestUser("u1")))
	assert.Empty(t, v1.GetAnnotation(other, v1.UserEncryptedAPIKeyAnnotation))

	ephemeral := cicdTestWorkload("ephemeral-1", common.CICDEphemeralRunnerKind, "u1")
	require.NoError(t, h.attachCICDEncryptedAPIKey(c, ephemeral, cicdTestUser("u1")))
	assert.Empty(t, v1.GetAnnotation(ephemeral, v1.UserEncryptedAPIKeyAnnotation))
}

func TestAttachCICDEncryptedAPIKeyRefusesUnresolvedKeys(t *testing.T) {
	setAuthMode(t, "builtin")
	h := &Handler{}
	c := cicdTestContext("")

	stubCICDPlatformKey(t, "ak-owner")
	err := h.attachCICDEncryptedAPIKey(c, cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "owner"), cicdTestUser("admin"))
	assert.True(t, apierrors.IsForbidden(err), "a non-owner cannot resolve the key")

	stubCICDPlatformKey(t, "")
	err = h.attachCICDEncryptedAPIKey(c, cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u1"), cicdTestUser("u1"))
	assert.True(t, apierrors.IsBadRequest(err), "an empty key is refused")
}

func TestAttachCICDEncryptedAPIKeyEncryptFailure(t *testing.T) {
	commonconfig.SetValue("crypto.enable", "false")
	t.Cleanup(func() { commonconfig.SetValue("crypto.enable", "") })
	if safecrypto.NewCrypto() == nil {
		t.Skip("the crypto client of this process is unavailable")
	}
	setAuthMode(t, "builtin")
	stubCICDPlatformKey(t, "ak-cicd-key")
	commonconfig.SetValue("crypto.enable", "true")

	workload := cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u1")
	err := (&Handler{}).attachCICDEncryptedAPIKey(cicdTestContext(""), workload, cicdTestUser("u1"))
	assert.True(t, apierrors.IsInternalError(err), "encryption without a key fails")
	assert.Empty(t, v1.GetAnnotation(workload, v1.UserEncryptedAPIKeyAnnotation))
}

func TestCICDPlatformKeyForUserWithoutDatabase(t *testing.T) {
	commonconfig.SetValue("db.enable", "false")
	t.Cleanup(func() { commonconfig.SetValue("db.enable", "") })
	_, err := cicdPlatformKeyForUser(context.Background(), "u1", "user-1")
	assert.True(t, apierrors.IsInternalError(err))
}
