/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// registryTestHandler builds an ImageHandler whose access controller allows the
// admin user (wildcard role) and whose dbClient is the supplied mock.
func registryTestHandler(t *testing.T, mockDB *mock_client.MockInterface) *ImageHandler {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	admin := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "u1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{"admin"}},
	}
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "admin"},
		Rules: []v1.PolicyRule{{
			Resources:    []string{"*"},
			GrantedUsers: []string{"*"},
			Verbs:        []v1.RoleVerb{"*"},
		}},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(admin, role).Build()
	return &ImageHandler{
		Client:           cl,
		dbClient:         mockDB,
		clientSet:        k8sfake.NewSimpleClientset(),
		accessController: &authority.AccessController{Client: cl},
	}
}

// ginCtx builds a gin context with the admin user id, optional JSON body, and params.
func ginCtx(t *testing.T, method, body string, params gin.Params) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, "/", strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, "/", nil)
	}
	r.Header.Set("Content-Type", "application/json")
	c.Request = r
	c.Set(common.UserId, "u1")
	c.Params = params
	return c
}

func TestListImageRegistry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListRegistryInfos(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*model.RegistryInfo{{ID: 1, Name: "r1", URL: "harbor.io", Username: ""}}, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodGet, "", nil)
	res, err := h.listImageRegistry(c)
	assert.NoError(t, err)
	assert.Len(t, res, 1)
	assert.Equal(t, "r1", res[0].Name)
}

func TestDeleteImageRegistry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetRegistryInfoById(gomock.Any(), int32(5)).Return(&model.RegistryInfo{ID: 5}, nil)
	m.EXPECT().DeleteRegistryInfo(gomock.Any(), int32(5)).Return(nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "5"}})
	_, err := h.deleteImageRegistry(c)
	assert.NoError(t, err)
}

func TestDeleteImageRegistryNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetRegistryInfoById(gomock.Any(), int32(5)).Return(nil, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "5"}})
	_, err := h.deleteImageRegistry(c)
	assert.NoError(t, err)
}

func TestDeleteImageRegistryBadID(t *testing.T) {
	h := registryTestHandler(t, nil)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "abc"}})
	_, err := h.deleteImageRegistry(c)
	assert.Error(t, err)
}

func TestCreateImageRegistryBadBody(t *testing.T) {
	h := registryTestHandler(t, nil)
	c := ginCtx(t, http.MethodPost, "{invalid", nil)
	_, err := h.createImageRegistry(c)
	assert.Error(t, err)
}

func TestCreateImageRegistryValidationError(t *testing.T) {
	h := registryTestHandler(t, nil)
	// Missing required fields -> Validate(true) fails before authorize.
	c := ginCtx(t, http.MethodPost, `{"name":"r1"}`, nil)
	_, err := h.createImageRegistry(c)
	assert.Error(t, err)
}

func TestUpdateImageRegistryBadID(t *testing.T) {
	h := registryTestHandler(t, nil)
	c := ginCtx(t, http.MethodPut, `{"name":"r1","url":"u","username":"a"}`, gin.Params{{Key: "id", Value: "x"}})
	_, err := h.updateImageRegistry(c)
	assert.Error(t, err)
}

func TestListImageRegistryAuthorizeFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	// User not present in client -> GetRequestUser fails -> authorize error.
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	h := &ImageHandler{Client: cl, dbClient: m, accessController: &authority.AccessController{Client: cl}}
	c := ginCtx(t, http.MethodGet, "", nil)
	_, err := h.listImageRegistry(c)
	assert.Error(t, err)
}
