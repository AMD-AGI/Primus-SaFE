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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestListUserHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	other := &v1.User{ObjectMeta: metav1.ObjectMeta{
		Name:        "other-user",
		Labels:      map[string]string{v1.UserIdLabel: "other-user"},
		Annotations: map[string]string{v1.UserNameAnnotation: "other-user"},
	}}
	h, user := newAdminHandlerWithObjects(other)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	h.ListUser(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var resp view.ListUserResponse
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
	// At least the admin user itself plus the other user.
	assert.GreaterOrEqual(t, resp.TotalCount, 1)
}

func TestGetUserSettingsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, common.UserSelf)
	h.GetUserSettings(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var resp view.UserSettingsResponse
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
}

func TestUpdateUserSettingsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	enable := true
	body, _ := json.Marshal(view.UpdateUserSettingsRequest{EnableNotification: &enable})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, common.UserSelf)
	h.UpdateUserSettings(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestDeleteUserHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	target := &v1.User{ObjectMeta: metav1.ObjectMeta{
		Name:        "to-delete",
		Labels:      map[string]string{v1.UserIdLabel: "to-delete"},
		Annotations: map[string]string{v1.UserNameAnnotation: "to-delete"},
	}}
	h, user := newAdminHandlerWithObjects(target)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "to-delete")
	h.DeleteUser(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestPatchUserHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	target := &v1.User{ObjectMeta: metav1.ObjectMeta{
		Name:        "to-patch",
		Labels:      map[string]string{v1.UserIdLabel: "to-patch"},
		Annotations: map[string]string{v1.UserNameAnnotation: "to-patch"},
	}}
	h, user := newAdminHandlerWithObjects(target)

	avatar := "http://new-avatar"
	body, _ := json.Marshal(view.PatchUserRequest{AvatarUrl: &avatar})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "to-patch")
	h.PatchUser(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestAuthUserUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	target := &v1.User{ObjectMeta: metav1.ObjectMeta{
		Name:        "target",
		Labels:      map[string]string{v1.UserIdLabel: "target"},
		Annotations: map[string]string{v1.UserNameAnnotation: "target"},
	}}
	h, user := newAdminHandlerWithObjects(target)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPatch, "/", nil)
	c.Set(common.UserId, user.Name)

	// Email change -> isChanged true (admin authorized).
	email := "new@example.com"
	changed, err := h.authUserUpdate(c, target, &view.PatchUserRequest{Email: &email})
	assert.NoError(t, err)
	assert.True(t, changed)

	// Empty request -> no change.
	changed, err = h.authUserUpdate(c, target, &view.PatchUserRequest{})
	assert.NoError(t, err)
	assert.False(t, changed)
}
