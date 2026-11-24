/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

func lowUtilSettings(exempt map[string]bool) *v1.UserLowUtilization {
	settings := &v1.UserLowUtilization{Workspaces: map[string]v1.UserLowUtilizationScope{}}
	for id, e := range exempt {
		settings.Workspaces[id] = v1.UserLowUtilizationScope{Exempt: e}
	}
	return settings
}

func storedLowUtil(t *testing.T, f *accessFixture, name string) *v1.UserLowUtilization {
	t.Helper()
	u := &v1.User{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: name}, u))
	return u.Spec.LowUtilization
}

func patchUserAs(t *testing.T, f *accessFixture, requester, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return qosCall(t, f.h, requester, target, http.MethodPatch, body, f.h.PatchUser)
}

func TestUserLowUtilizationPatchNeedsSystemAdmin(t *testing.T) {
	f := newAccessFixture(t)
	body := view.PatchUserRequest{LowUtilization: lowUtilSettings(map[string]bool{"ws1": true})}
	for _, requester := range []string{userDefault, userReadonly} {
		rsp := patchUserAs(t, f, requester, userDefault, body)
		assert.Equal(t, http.StatusForbidden, rsp.Code, "%s: %s", requester, rsp.Body.String())
	}
	assert.Nil(t, storedLowUtil(t, f, userDefault))
}

func TestUserLowUtilizationPatchStoresExemptOnly(t *testing.T) {
	f := newAccessFixture(t)
	rsp := patchUserAs(t, f, userAdmin, userDefault, view.PatchUserRequest{
		LowUtilization: lowUtilSettings(map[string]bool{"ws1": true, "ws2": false, "not-created-yet": true}),
	})
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	assert.Equal(t, lowUtilSettings(map[string]bool{"ws1": true, "not-created-yet": true}), storedLowUtil(t, f, userDefault))

	rsp = patchUserAs(t, f, userAdmin, userDefault, map[string]any{"email": "default@example.com"})
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	assert.Equal(t, lowUtilSettings(map[string]bool{"ws1": true, "not-created-yet": true}), storedLowUtil(t, f, userDefault),
		"a patch without lowUtilization keeps it")

	rsp = patchUserAs(t, f, userAdmin, userDefault, view.PatchUserRequest{
		LowUtilization: lowUtilSettings(map[string]bool{"ws1": false}),
	})
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	assert.Nil(t, storedLowUtil(t, f, userDefault))

	rsp = patchUserAs(t, f, userAdmin, userDefault, view.PatchUserRequest{
		LowUtilization: lowUtilSettings(map[string]bool{"ws1": true}),
	})
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	rsp = patchUserAs(t, f, userAdmin, userDefault, map[string]any{"lowUtilization": map[string]any{}})
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	assert.Nil(t, storedLowUtil(t, f, userDefault))
}

func TestUserLowUtilizationPatchValidation(t *testing.T) {
	f := newAccessFixture(t)
	for name, settings := range map[string]*v1.UserLowUtilization{
		"empty workspace id": lowUtilSettings(map[string]bool{"": true}),
		"blank workspace id": lowUtilSettings(map[string]bool{"  ": true}),
	} {
		rsp := patchUserAs(t, f, userAdmin, userDefault, view.PatchUserRequest{LowUtilization: settings})
		assert.Equal(t, http.StatusBadRequest, rsp.Code, "%s: %s", name, rsp.Body.String())
	}

	maxed := map[string]bool{}
	for i := 0; i < view.MaxUserLowUtilizationWorkspaces; i++ {
		maxed[fmt.Sprintf("ws-%d", i)] = true
	}
	assert.NoError(t, view.ValidateUserLowUtilization(lowUtilSettings(maxed)))
	maxed["one-more"] = false
	rsp := patchUserAs(t, f, userAdmin, userDefault, view.PatchUserRequest{LowUtilization: lowUtilSettings(maxed)})
	assert.Equal(t, http.StatusBadRequest, rsp.Code, rsp.Body.String())
	assert.Nil(t, storedLowUtil(t, f, userDefault))
	assert.NoError(t, view.ValidateUserLowUtilization(nil))
}

// listUsersAs runs GET /users as the given requester and returns the items by user id.
func listUsersAs(t *testing.T, h *Handler, requester string) map[string]map[string]any {
	t.Helper()
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, requester)
	h.ListUser(c)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	var got struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
	byID := make(map[string]map[string]any, len(got.Items))
	for _, item := range got.Items {
		byID[item["id"].(string)] = item
	}
	return byID
}

func TestUserLowUtilizationVisibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	exemptUser := func() *v1.User {
		u := genPlainUser()
		u.Spec.LowUtilization = lowUtilSettings(map[string]bool{"ws1": true})
		commonuser.AssignWorkspace(u, "ws1")
		return u
	}
	peer := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "peer-user",
			Labels: map[string]string{v1.UserIdLabel: "peer-user"},
		},
		Spec: v1.UserSpec{
			Type:           v1.DefaultUserType,
			Roles:          []v1.UserRole{"ws-member"},
			LowUtilization: lowUtilSettings(map[string]bool{"ws1": true}),
		},
	}
	commonuser.AssignWorkspace(peer, "ws1")

	t.Run("system admin sees another user's settings", func(t *testing.T) {
		target := exemptUser()
		h, admin := newAdminHandlerWithObjects(target)
		assert.NotNil(t, getUserAs(t, h, admin.Name, target.Name)["lowUtilization"])
		assert.NotNil(t, listUsersAs(t, h, admin.Name)[target.Name]["lowUtilization"])
	})

	t.Run("user sees own settings", func(t *testing.T) {
		self := exemptUser()
		self.Spec.Roles = []v1.UserRole{"ws-member"}
		h := newWorkspacesHandler(self, genMemberRole())
		assert.NotNil(t, getUserAs(t, h, self.Name, common.UserSelf)["lowUtilization"])
		assert.NotNil(t, listUsersAs(t, h, self.Name)[self.Name]["lowUtilization"])
	})

	t.Run("workspace peer does not see the settings", func(t *testing.T) {
		target := exemptUser()
		h := newWorkspacesHandler(peer.DeepCopy(), target, genMemberRole())
		got := getUserAs(t, h, peer.Name, target.Name)
		assert.Nil(t, got["lowUtilization"])
		assert.Nil(t, got["qos"])
		listed := listUsersAs(t, h, peer.Name)
		require.Contains(t, listed, target.Name)
		assert.Nil(t, listed[target.Name]["lowUtilization"])
		assert.NotNil(t, listed[peer.Name]["lowUtilization"])
	})
}
