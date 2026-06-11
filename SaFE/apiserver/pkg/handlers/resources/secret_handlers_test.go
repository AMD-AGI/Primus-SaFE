/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// newSecretHandlerWithCtrlSecrets seeds secrets into BOTH the controller-runtime
// client (used by listSecret/patchSecret) and the k8s clientSet, with corev1
// registered in the scheme.
func newSecretHandlerWithCtrlSecrets(secrets ...*corev1.Secret) (*Handler, *v1.User) {
	mockUser := genMockUser()
	mockRole := genMockRole()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	crObjs := []client.Object{mockUser, mockRole}
	k8sObjs := []runtime.Object{}
	for _, s := range secrets {
		crObjs = append(crObjs, s)
		k8sObjs = append(k8sObjs, s)
	}
	ctrlClient := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(crObjs...).Build()
	h := &Handler{
		Client:           ctrlClient,
		clientSet:        k8sfake.NewSimpleClientset(k8sObjs...),
		accessController: authority.NewAccessController(ctrlClient),
	}
	return h, mockUser
}

// seedGeneralSecret builds a general k8s secret in the primus-safe namespace.
func seedGeneralSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{v1.SecretTypeLabel: string(v1.SecretGeneral)},
		},
		Data: map[string][]byte{"token": []byte("abc")},
	}
}

func TestCreateSecretHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newSecretHandler()

	body, _ := json.Marshal(view.CreateSecretRequest{
		Name:   "s-new",
		Type:   v1.SecretGeneral,
		Params: []map[view.SecretParam]string{{view.SecretParam("k"): b64("v")}},
	})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	h.CreateSecret(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	// Missing required fields -> bad request.
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"name":"x"}`)))
	c2.Request.Header.Set("Content-Type", "application/json")
	c2.Set(common.UserId, user.Name)
	h.CreateSecret(c2)
	assert.NotEqual(t, http.StatusOK, rsp2.Code)
}

func TestListSecretHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newSecretHandlerWithCtrlSecrets(seedGeneralSecret("s1"), seedGeneralSecret("s2"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	h.ListSecret(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var resp view.ListSecretResponse
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.TotalCount)
}

func TestGetSecretHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newSecretHandler(seedGeneralSecret("s1"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "s1")
	h.GetSecret(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestDeleteSecretHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newSecretHandler(seedGeneralSecret("s-del"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "s-del")
	h.DeleteSecret(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestPatchSecretHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newSecretHandlerWithCtrlSecrets(seedGeneralSecret("s1"))

	newParams := []map[view.SecretParam]string{{view.SecretParam("token"): b64("new-val")}}
	body, _ := json.Marshal(view.PatchSecretRequest{Params: &newParams})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "s1")
	h.PatchSecret(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}
