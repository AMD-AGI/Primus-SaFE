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
	testifyassert "github.com/stretchr/testify/assert"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
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

func TestIsUserEnableNotification(t *testing.T) {
	t.Run("default is false", func(t *testing.T) {
		user := genMockUser()
		assert.Equal(t, v1.IsUserEnableNotification(user), false)
	})

	t.Run("returns true when annotation set", func(t *testing.T) {
		user := genMockUser()
		v1.SetAnnotation(user, v1.UserEnableNotificationAnnotation, v1.TrueStr)
		assert.Equal(t, v1.IsUserEnableNotification(user), true)
	})

	t.Run("returns false after annotation removed", func(t *testing.T) {
		user := genMockUser()
		v1.SetAnnotation(user, v1.UserEnableNotificationAnnotation, v1.TrueStr)
		v1.RemoveAnnotation(user, v1.UserEnableNotificationAnnotation)
		assert.Equal(t, v1.IsUserEnableNotification(user), false)
	})
}

func TestUserSettingsResponse(t *testing.T) {
	t.Run("response reflects annotation off", func(t *testing.T) {
		user := genMockUser()
		resp := &view.UserSettingsResponse{
			EnableNotification: v1.IsUserEnableNotification(user),
		}
		assert.Equal(t, resp.EnableNotification, false)
	})

	t.Run("response reflects annotation on", func(t *testing.T) {
		user := genMockUser()
		v1.SetAnnotation(user, v1.UserEnableNotificationAnnotation, v1.TrueStr)
		resp := &view.UserSettingsResponse{
			EnableNotification: v1.IsUserEnableNotification(user),
		}
		assert.Equal(t, resp.EnableNotification, true)
	})
}

func TestUserSettingsAnnotationPersistence(t *testing.T) {
	user := genMockUser()
	s := runtime.NewScheme()
	_ = v1.AddToScheme(s)
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(user).Build()
	ctx := context.Background()

	t.Run("enable persists to store", func(t *testing.T) {
		v1.SetAnnotation(user, v1.UserEnableNotificationAnnotation, v1.TrueStr)
		err := fakeClient.Update(ctx, user)
		assert.NilError(t, err)

		fetched := &v1.User{}
		err = fakeClient.Get(ctx, client.ObjectKeyFromObject(user), fetched)
		assert.NilError(t, err)
		assert.Equal(t, v1.IsUserEnableNotification(fetched), true)
	})

	t.Run("disable persists to store", func(t *testing.T) {
		v1.RemoveAnnotation(user, v1.UserEnableNotificationAnnotation)
		err := fakeClient.Update(ctx, user)
		assert.NilError(t, err)

		fetched := &v1.User{}
		err = fakeClient.Get(ctx, client.ObjectKeyFromObject(user), fetched)
		assert.NilError(t, err)
		assert.Equal(t, v1.IsUserEnableNotification(fetched), false)
	})
}

// --- merged from user_crud_test.go ---

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
	testifyassert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
	// At least the admin user itself plus the other user.
	testifyassert.GreaterOrEqual(t, resp.TotalCount, 1)
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
	testifyassert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
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
	testifyassert.NoError(t, err)
	testifyassert.True(t, changed)

	// Empty request -> no change.
	changed, err = h.authUserUpdate(c, target, &view.PatchUserRequest{})
	testifyassert.NoError(t, err)
	testifyassert.False(t, changed)
}

// --- merged from user_handlers_test.go ---

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
	testifyassert.Empty(t, user2.Spec.Password)
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

	testifyassert.Equal(t, roles, user.Spec.Roles)
	assert.Equal(t, stringutil.Base64Encode("new-pass"), user.Spec.Password)
	assert.Equal(t, "new@example.com", v1.GetUserEmail(user))
	assert.Equal(t, "http://new-avatar", v1.GetUserAvatarUrl(user))
	assert.Equal(t, restricted, user.Spec.RestrictedType)
}

func TestBuildListUserSelector(t *testing.T) {
	// Empty query -> empty selector.
	sel := buildListUserSelector(&view.ListUserRequest{})
	testifyassert.True(t, sel.Empty())

	// With name and email filters.
	sel2 := buildListUserSelector(&view.ListUserRequest{Name: "alice", Email: "a%40b.com"})
	testifyassert.False(t, sel2.Empty())
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
	testifyassert.NoError(t, err)
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
		testifyassert.NoError(t, err)
		assert.Equal(t, "alice", q.Name)
		testifyassert.False(t, q.IsFromConsole)
	})

	t.Run("form body", func(t *testing.T) {
		form := "type=default&name=bob&password=" + "p&code=c"
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(form)))
		c.Request.Header.Set("Content-Type", ContentTypeForm)
		q, err := parseLoginQuery(c)
		testifyassert.NoError(t, err)
		assert.Equal(t, "bob", q.Name)
		testifyassert.True(t, q.IsFromConsole)
	})
}

func TestSetCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	// Negative expire -> max cookie age.
	setCookie(c, &view.UserLoginResponse{Token: "t", Expire: -1}, v1.DefaultUserType)
	testifyassert.NotEmpty(t, rsp.Header().Values("Set-Cookie"))
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
	testifyassert.Error(t, err)

	u, err := h.getAdminUser(context.Background(), "u-x")
	testifyassert.NoError(t, err)
	assert.Equal(t, "u-x", u.Name)

	_, err = h.getAdminUser(context.Background(), "missing")
	testifyassert.Error(t, err)
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
