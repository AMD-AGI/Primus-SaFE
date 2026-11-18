/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestResolveCICDAPIKeyPlainExternalUsesBearerAPIKey(t *testing.T) {
	setAuthMode(t, "external")
	c := cicdTestContext("Bearer ak-from-header")

	got, err := resolveCICDAPIKeyPlain(c, cicdTestWorkload("cicd-1", common.CICDScaleRunnerSetKind, "u1"), cicdTestUser("u1"))
	require.NoError(t, err)
	assert.Equal(t, "ak-from-header", got)
}

func TestResolveCICDAPIKeyPlainExternalNeedsSession(t *testing.T) {
	setAuthMode(t, "external")
	c := cicdTestContext("")

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
