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
	"time"

	"github.com/gin-gonic/gin"
	testifyassert "github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
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
func genDockerConfigData(t *testing.T, server, username, pwd string) []byte {
	config := view.DockerConfig{
		Auths: map[string]view.DockerConfigItem{
			server: {
				UserName: username,
				Password: pwd,
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

// --- merged from secret_handlers_test.go ---

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
	testifyassert.NotEqual(t, http.StatusOK, rsp2.Code)
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
	testifyassert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
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

// --- merged from secret_more_test.go ---

func b64(s string) string { return stringutil.Base64Encode(s) }

func TestExistKey(t *testing.T) {
	params := map[view.SecretParam]string{view.UserNameParam: "u", view.PasswordParam: ""}
	testifyassert.True(t, existKey(params, view.UserNameParam))
	testifyassert.False(t, existKey(params, view.PasswordParam))
	testifyassert.False(t, existKey(params, view.ServerParam))
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
		testifyassert.NoError(t, err)
		assert.Equal(t, corev1.SecretTypeDockerConfigJson, secret.Type)
		testifyassert.NotEmpty(t, secret.Data[view.DockerConfigJson])
	})

	t.Run("image missing field", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{view.ServerParam: "r"}}
		testifyassert.Error(t, buildSecretData(v1.SecretImage, params, secret))
	})

	t.Run("ssh with password", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{
			view.UserNameParam: "user",
			view.PasswordParam: b64("secret"),
		}}
		testifyassert.NoError(t, buildSecretData(v1.SecretSSH, params, secret))
		assert.Equal(t, corev1.SecretTypeOpaque, secret.Type)
	})

	t.Run("ssh with keypair", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{
			view.UserNameParam:   "user",
			view.PrivateKeyParam: b64("priv"),
			view.PublicKeyParam:  b64("pub"),
		}}
		testifyassert.NoError(t, buildSecretData(v1.SecretSSH, params, secret))
		testifyassert.NotEmpty(t, secret.Data[view.SSHAuthKey])
	})

	t.Run("ssh missing creds", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{view.UserNameParam: "user"}}
		testifyassert.Error(t, buildSecretData(v1.SecretSSH, params, secret))
	})

	t.Run("general", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{view.SecretParam("token"): b64("abc")}}
		testifyassert.NoError(t, buildSecretData(v1.SecretGeneral, params, secret))
		assert.Equal(t, []byte("abc"), secret.Data["token"])
	})

	t.Run("empty params", func(t *testing.T) {
		secret := &corev1.Secret{}
		testifyassert.Error(t, buildSecretData(v1.SecretSSH, nil, secret))
	})

	t.Run("unsupported type", func(t *testing.T) {
		secret := &corev1.Secret{}
		params := []map[view.SecretParam]string{{view.UserNameParam: "u"}}
		testifyassert.Error(t, buildSecretData(v1.SecretType("weird"), params, secret))
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
	testifyassert.NoError(t, err)
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
	testifyassert.Len(t, resp.Params, 1)
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
		testifyassert.NoError(t, err)
		assert.Equal(t, "s1", req.Name)
	})

	t.Run("missing fields", func(t *testing.T) {
		body, _ := json.Marshal(view.CreateSecretRequest{Name: "s1"})
		c := newPostCtx(body)
		_, err := parseCreateSecretRequest(c)
		testifyassert.Error(t, err)
	})
}

// newSecretHandler builds a Handler with a k8s fake clientSet and admin RBAC.
func newSecretHandler(objs ...runtime.Object) (*Handler, *v1.User) {
	mockUser := genMockUser()
	mockRole := genMockRole()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	fakeCtrl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(mockUser, mockRole).Build()
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
	testifyassert.Error(t, err)

	got, err := h.getAdminSecret(context.Background(), "s1")
	testifyassert.NoError(t, err)
	assert.Equal(t, "s1", got.Name)

	_, err = h.getAdminSecret(context.Background(), "missing")
	testifyassert.Error(t, err)
}

func TestCreateAndDeleteSecretImpl(t *testing.T) {
	h, user := newSecretHandler()

	req := &view.CreateSecretRequest{
		Name:   "s-create",
		Type:   v1.SecretGeneral,
		Params: []map[view.SecretParam]string{{view.SecretParam("k"): b64("v")}},
	}
	secret, err := h.createSecretImpl(context.Background(), req, user)
	testifyassert.NoError(t, err)
	assert.Equal(t, "s-create", secret.Name)

	// Delete the secret just created.
	err = h.deleteSecretImpl(context.Background(), "s-create", user)
	testifyassert.NoError(t, err)
}

// newPostCtx builds a gin context with a JSON body for POST requests.
func newPostCtx(body []byte) *gin.Context {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}
