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
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func genMockUser() *v1.User {
	return &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-user",
			Labels: map[string]string{
				v1.UserIdLabel: "test-user",
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: "test-user",
			},
		},
		Spec: v1.UserSpec{
			Type:  v1.DefaultUserType,
			Roles: []v1.UserRole{v1.SystemAdminRole},
		},
	}
}

func genMockRole() *v1.Role {
	return &v1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name: string(v1.SystemAdminRole),
		},
		Rules: []v1.PolicyRule{{
			Resources:    []string{authority.AllResource},
			Verbs:        []v1.RoleVerb{v1.AllVerb},
			GrantedUsers: []string{authority.GrantedAllUser},
		}},
	}
}

func createMockUser() (*v1.User, client.WithWatch) {
	mockUser := genMockUser()
	mockRole := genMockRole()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockUser, mockRole).Build()
	return mockUser, fakeClient
}

func TestUserSettings(t *testing.T) {
	user := genMockUser()
	role := genMockRole()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(user, role).Build()
	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

	t.Run("default is off", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, common.UserSelf)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users/self/settings", nil)

		h.GetUserSettings(c)
		assert.Equal(t, rsp.Code, http.StatusOK)

		var result view.UserSettingsResponse
		err := json.Unmarshal(rsp.Body.Bytes(), &result)
		assert.NilError(t, err)
		assert.Equal(t, result.EnableNotification, false)
	})

	t.Run("enable notification", func(t *testing.T) {
		enable := true
		body, _ := json.Marshal(view.UpdateUserSettingsRequest{EnableNotification: &enable})
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, common.UserSelf)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/users/self/settings", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.UpdateUserSettings(c)
		assert.Equal(t, rsp.Code, http.StatusOK)

		updated := &v1.User{}
		err := fakeClient.Get(c.Request.Context(), client.ObjectKeyFromObject(user), updated)
		assert.NilError(t, err)
		assert.Equal(t, v1.IsUserEnableNotification(updated), true)
	})

	t.Run("get returns enabled", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, common.UserSelf)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users/self/settings", nil)

		h.GetUserSettings(c)
		assert.Equal(t, rsp.Code, http.StatusOK)

		var result view.UserSettingsResponse
		err := json.Unmarshal(rsp.Body.Bytes(), &result)
		assert.NilError(t, err)
		assert.Equal(t, result.EnableNotification, true)
	})

	t.Run("disable notification", func(t *testing.T) {
		disable := false
		body, _ := json.Marshal(view.UpdateUserSettingsRequest{EnableNotification: &disable})
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, common.UserSelf)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/users/self/settings", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.UpdateUserSettings(c)
		assert.Equal(t, rsp.Code, http.StatusOK)

		updated := &v1.User{}
		err := fakeClient.Get(c.Request.Context(), client.ObjectKeyFromObject(user), updated)
		assert.NilError(t, err)
		assert.Equal(t, v1.IsUserEnableNotification(updated), false)
	})

	t.Run("get returns disabled", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, common.UserSelf)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users/self/settings", nil)

		h.GetUserSettings(c)
		assert.Equal(t, rsp.Code, http.StatusOK)

		var result view.UserSettingsResponse
		err := json.Unmarshal(rsp.Body.Bytes(), &result)
		assert.NilError(t, err)
		assert.Equal(t, result.EnableNotification, false)
	})
}
