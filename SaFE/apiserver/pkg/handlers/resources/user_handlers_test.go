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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func TestGenerateUser(t *testing.T) {
	req := &view.CreateUserRequest{
		Name:      "alice",
		Email:     "alice@example.com",
		Password:  "secret",
		AvatarUrl: "http://avatar",
	}
	user := generateUser(req)
	assert.Equal(t, "alice", v1.GetUserName(user))
	assert.Equal(t, "alice@example.com", v1.GetUserEmail(user))
	assert.Equal(t, v1.DefaultUserType, user.Spec.Type)
	assert.Equal(t, stringutil.Base64Encode("secret"), user.Spec.Password)

	// No password leaves the field empty.
	user2 := generateUser(&view.CreateUserRequest{Name: "bob"})
	assert.Empty(t, user2.Spec.Password)
}

func TestApplyUserPatch(t *testing.T) {
	user := genMockUser()
	roles := []v1.UserRole{v1.DefaultRole}
	ws := []string{"ws-1"}
	avatar := "http://new-avatar"
	pwd := "new-pass"
	email := "new@example.com"
	restricted := v1.UserRestrictedType(1)

	req := &view.PatchUserRequest{
		Roles:          &roles,
		Workspaces:     &ws,
		AvatarUrl:      &avatar,
		Email:          &email,
		RestrictedType: &restricted,
	}
	req.Password = &pwd
	applyUserPatch(user, req)

	assert.Equal(t, roles, user.Spec.Roles)
	assert.Equal(t, stringutil.Base64Encode("new-pass"), user.Spec.Password)
	assert.Equal(t, "new@example.com", v1.GetUserEmail(user))
	assert.Equal(t, "http://new-avatar", v1.GetUserAvatarUrl(user))
	assert.Equal(t, restricted, user.Spec.RestrictedType)
}

func TestBuildListUserSelector(t *testing.T) {
	// Empty query -> empty selector.
	sel := buildListUserSelector(&view.ListUserRequest{})
	assert.True(t, sel.Empty())

	// With name and email filters.
	sel2 := buildListUserSelector(&view.ListUserRequest{Name: "alice", Email: "a%40b.com"})
	assert.False(t, sel2.Empty())
}

func TestQueryUnescape(t *testing.T) {
	assert.Equal(t, "a@b.com", queryUnescape("a%40b.com"))
	// Invalid escape returns the original string.
	assert.Equal(t, "%zz", queryUnescape("%zz"))
}

func TestParseListUserQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/?name=alice&workspaceId=ws-1", nil)
	q, err := parseListUserQuery(c)
	assert.NoError(t, err)
	assert.Equal(t, "alice", q.Name)
	assert.Equal(t, "ws-1", q.WorkspaceId)
}

func TestParseLoginQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("json body", func(t *testing.T) {
		body, _ := json.Marshal(view.UserLoginRequest{Name: "alice", Password: "p", Type: v1.DefaultUserType})
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		q, err := parseLoginQuery(c)
		assert.NoError(t, err)
		assert.Equal(t, "alice", q.Name)
		assert.False(t, q.IsFromConsole)
	})

	t.Run("form body", func(t *testing.T) {
		form := "type=default&name=bob&password=" + "p&code=c"
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(form)))
		c.Request.Header.Set("Content-Type", ContentTypeForm)
		q, err := parseLoginQuery(c)
		assert.NoError(t, err)
		assert.Equal(t, "bob", q.Name)
		assert.True(t, q.IsFromConsole)
	})
}

func TestSetCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	// Negative expire -> max cookie age.
	setCookie(c, &view.UserLoginResponse{Token: "t", Expire: -1}, v1.DefaultUserType)
	assert.NotEmpty(t, rsp.Header().Values("Set-Cookie"))
}

func TestLogout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	h.Logout(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetAdminUser(t *testing.T) {
	h, _ := newAdminHandlerWithObjects(&v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u-x"}})

	_, err := h.getAdminUser(context.Background(), "")
	assert.Error(t, err)

	u, err := h.getAdminUser(context.Background(), "u-x")
	assert.NoError(t, err)
	assert.Equal(t, "u-x", u.Name)

	_, err = h.getAdminUser(context.Background(), "missing")
	assert.Error(t, err)
}

func TestCvtToUserResponseItem(t *testing.T) {
	h, user := newAdminHandlerWithObjects()
	// Admin user: no workspaces resolved.
	item := h.cvtToUserResponseItem(context.Background(), user)
	assert.Equal(t, user.Name, item.Id)
	assert.Equal(t, user.Spec.Type, item.Type)
}

func TestGetUserHandlerSelf(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, common.UserSelf)
	h.GetUser(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestCreateUserHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := newAdminHandlerWithObjects()

	body, _ := json.Marshal(view.CreateUserRequest{Name: "newbie", Password: "p"})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateUser(c)
	// SSO disabled in test env -> user created successfully.
	assert.Equal(t, http.StatusOK, rsp.Code)
}
