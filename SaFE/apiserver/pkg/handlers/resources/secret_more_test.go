/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"k8s.io/apimachinery/pkg/runtime"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func b64(s string) string { return stringutil.Base64Encode(s) }

func TestExistKey(t *testing.T) {
	params := map[view.SecretParam]string{view.UserNameParam: "u", view.PasswordParam: ""}
	assert.True(t, existKey(params, view.UserNameParam))
	assert.False(t, existKey(params, view.PasswordParam))
	assert.False(t, existKey(params, view.ServerParam))
}

func TestBuildSecretData(t *testing.T) {
	t.Run("image", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{
			view.ServerParam:   "registry.io",
			view.UserNameParam: "user",
			view.PasswordParam: b64("pass"),
		}}
		err := buildSecretData(v1.SecretImage, params, secret)
		assert.NoError(t, err)
		assert.Equal(t, corev1.SecretTypeDockerConfigJson, secret.Type)
		assert.NotEmpty(t, secret.Data[view.DockerConfigJson])
	})

	t.Run("image missing field", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{view.ServerParam: "r"}}
		assert.Error(t, buildSecretData(v1.SecretImage, params, secret))
	})

	t.Run("ssh with password", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{
			view.UserNameParam: "user",
			view.PasswordParam: b64("secret"),
		}}
		assert.NoError(t, buildSecretData(v1.SecretSSH, params, secret))
		assert.Equal(t, corev1.SecretTypeOpaque, secret.Type)
	})

	t.Run("ssh with keypair", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{
			view.UserNameParam:   "user",
			view.PrivateKeyParam: b64("priv"),
			view.PublicKeyParam:  b64("pub"),
		}}
		assert.NoError(t, buildSecretData(v1.SecretSSH, params, secret))
		assert.NotEmpty(t, secret.Data[view.SSHAuthKey])
	})

	t.Run("ssh missing creds", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{view.UserNameParam: "user"}}
		assert.Error(t, buildSecretData(v1.SecretSSH, params, secret))
	})

	t.Run("general", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{view.SecretParam("token"): b64("abc")}}
		assert.NoError(t, buildSecretData(v1.SecretGeneral, params, secret))
		assert.Equal(t, []byte("abc"), secret.Data["token"])
	})

	t.Run("empty params", func(t *testing.T) {
		secret := &corev1.Secret{}
		assert.Error(t, buildSecretData(v1.SecretSSH, nil, secret))
	})

	t.Run("unsupported type", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{view.UserNameParam: "u"}}
		assert.Error(t, buildSecretData(v1.SecretType("weird"), params, secret))
	})
}

func TestGenerateSecret(t *testing.T) {
	user := genMockUser()
	req := &view.CreateSecretRequest{
		Name: "my-secret",
		Type: v1.SecretGeneral,
		Params: []map[view.SecretParam]string{
			{view.SecretParam("token"): b64("abc")},
		},
		WorkspaceIds: []string{"ws-1"},
		Owner:        "owner-1",
		Labels:       map[string]string{"team": "infra"},
	}
	secret, err := generateSecret(req, user)
	assert.NoError(t, err)
	assert.Equal(t, "my-secret", secret.Name)
	assert.Equal(t, "infra", secret.Labels["team"])
}

func TestCvtToGetSecretResponse(t *testing.T) {
	// General secret round-trip.
	secret := &corev1.Secret{}
	secret.Labels = map[string]string{v1.SecretTypeLabel: string(v1.SecretGeneral), "team": "infra"}
	secret.Data = map[string][]byte{"token": []byte("abc")}
	resp := cvtToGetSecretResponse(secret)
	assert.Equal(t, string(v1.SecretGeneral), resp.Type)
	assert.Len(t, resp.Params, 1)
	assert.Equal(t, "infra", resp.Labels["team"])
}

func TestParseCreateSecretRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("valid", func(t *testing.T) {
		body, _ := json.Marshal(view.CreateSecretRequest{
			Name:   "s1",
			Type:   v1.SecretGeneral,
			Params: []map[view.SecretParam]string{{view.SecretParam("k"): "v"}},
		})
		c := newPostCtx(body)
		req, err := parseCreateSecretRequest(c)
		assert.NoError(t, err)
		assert.Equal(t, "s1", req.Name)
	})

	t.Run("missing fields", func(t *testing.T) {
		body, _ := json.Marshal(view.CreateSecretRequest{Name: "s1"})
		c := newPostCtx(body)
		_, err := parseCreateSecretRequest(c)
		assert.Error(t, err)
	})
}

// newSecretHandler builds a Handler with a k8s fake clientSet and admin RBAC.
func newSecretHandler(objs ...runtime.Object) (*Handler, *v1.User) {
	mockUser := genMockUser()
	mockRole := genMockRole()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	fakeCtrl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockUser, mockRole).Build()
	h := &Handler{
		Client:           fakeCtrl,
		clientSet:        k8sfake.NewSimpleClientset(objs...),
		accessController: authority.NewAccessController(fakeCtrl),
	}
	return h, mockUser
}

func TestGetAdminSecret(t *testing.T) {
	existing := &corev1.Secret{}
	existing.Name = "s1"
	existing.Namespace = common.PrimusSafeNamespace
	h, _ := newSecretHandler(existing)

	_, err := h.getAdminSecret(context.Background(), "")
	assert.Error(t, err)

	got, err := h.getAdminSecret(context.Background(), "s1")
	assert.NoError(t, err)
	assert.Equal(t, "s1", got.Name)

	_, err = h.getAdminSecret(context.Background(), "missing")
	assert.Error(t, err)
}

func TestCreateAndDeleteSecretImpl(t *testing.T) {
	h, user := newSecretHandler()

	req := &view.CreateSecretRequest{
		Name:   "s-create",
		Type:   v1.SecretGeneral,
		Params: []map[view.SecretParam]string{{view.SecretParam("k"): b64("v")}},
	}
	secret, err := h.createSecretImpl(context.Background(), req, user)
	assert.NoError(t, err)
	assert.Equal(t, "s-create", secret.Name)

	// Delete the secret just created.
	err = h.deleteSecretImpl(context.Background(), "s-create", user)
	assert.NoError(t, err)
}

// newPostCtx builds a gin context with a JSON body for POST requests.
func newPostCtx(body []byte) *gin.Context {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}
