/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

func TestResolveCICDAPIKeyPlainUsesBearerAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)
	c.Request.Header.Set("Authorization", "Bearer ak-from-header")

	got, err := resolveCICDAPIKeyPlain(c, &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}})
	require.NoError(t, err)
	assert.Equal(t, "ak-from-header", got)
}

func TestAttachCICDEncryptedAPIKey(t *testing.T) {
	commonconfig.SetValue("crypto.enable", "false")
	t.Cleanup(func() { commonconfig.SetValue("crypto.enable", "") })

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)
	c.Request.Header.Set("Authorization", "Bearer ak-cicd-key")
	c.Set(common.UserType, "sso")

	workload := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "cicd-1"}}
	workload.Spec.GroupVersionKind.Kind = common.CICDScaleRunnerSetKind
	h := &Handler{}
	require.NoError(t, h.attachCICDEncryptedAPIKey(c, workload, &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}}))
	assert.Equal(t, "ak-cicd-key", v1.GetAnnotation(workload, v1.UserEncryptedAPIKeyAnnotation))

	other := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "train-1"}}
	other.Spec.GroupVersionKind.Kind = common.PytorchJobKind
	require.NoError(t, h.attachCICDEncryptedAPIKey(c, other, &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}}))
	assert.Empty(t, v1.GetAnnotation(other, v1.UserEncryptedAPIKeyAnnotation))
}

func TestResolveCICDAPIKeyPlainExternalNeedsSession(t *testing.T) {
	prev := viper.Get("auth.mode")
	viper.Set("auth.mode", "external")
	t.Cleanup(func() { viper.Set("auth.mode", prev) })

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)

	_, err := resolveCICDAPIKeyPlain(c, &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}})
	require.Error(t, err)
}
