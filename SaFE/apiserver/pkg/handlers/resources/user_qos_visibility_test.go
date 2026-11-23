/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	testifyassert "github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// genPlainUser returns a user without system administrator roles.
func genPlainUser() *v1.User {
	user := genMockUser()
	user.Name = "plain-user"
	user.Labels[v1.UserIdLabel] = "plain-user"
	user.Annotations[v1.UserNameAnnotation] = "plain-user"
	user.Spec.Roles = nil
	user.Spec.Qos = &v1.UserQos{MaxResources: &v1.ResourceCap{GPU: "4"}}
	return user
}

// genMemberRole grants read access on users to every signed-in user.
func genMemberRole() *v1.Role {
	return &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-member"},
		Rules: []v1.PolicyRule{{
			Resources:    []string{"user"},
			Verbs:        []v1.RoleVerb{v1.GetVerb, v1.ListVerb},
			GrantedUsers: []string{authority.GrantedAllUser},
		}},
	}
}

// getUserAs runs GET /users/:id as the given requester and returns the decoded body.
func getUserAs(t *testing.T, h *Handler, requester, target string) map[string]interface{} {
	t.Helper()
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, requester)
	c.Set(common.Name, target)
	h.GetUser(c)
	testifyassert.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())

	var got map[string]interface{}
	testifyassert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
	return got
}

func TestGetUserQosVisibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	plain := genPlainUser()

	t.Run("system admin sees another user's qos", func(t *testing.T) {
		h, _ := newAdminHandlerWithObjects(plain)
		got := getUserAs(t, h, "test-user", plain.Name)
		testifyassert.NotNil(t, got["qos"])
	})

	t.Run("user sees own qos", func(t *testing.T) {
		self := genPlainUser()
		self.Spec.Roles = []v1.UserRole{"ws-member"}
		h := newWorkspacesHandler(self, genMemberRole())
		got := getUserAs(t, h, self.Name, common.UserSelf)
		testifyassert.NotNil(t, got["qos"])
	})

	t.Run("workspace peer does not see qos", func(t *testing.T) {
		peer := &v1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "peer-user",
				Labels: map[string]string{v1.UserIdLabel: "peer-user"},
			},
			Spec: v1.UserSpec{
				Type:  v1.DefaultUserType,
				Roles: []v1.UserRole{"ws-member"},
			},
		}
		commonuser.AssignWorkspace(peer, "ws1")
		target := genPlainUser()
		commonuser.AssignWorkspace(target, "ws1")
		h := newWorkspacesHandler(peer, target, genMemberRole())
		got := getUserAs(t, h, peer.Name, target.Name)
		testifyassert.Nil(t, got["qos"])
	})
}

func TestIsSystemAdminRequest(t *testing.T) {
	h, admin := newAdminHandlerWithObjects(genPlainUser())
	ctx := context.Background()
	testifyassert.True(t, h.isSystemAdminRequest(ctx, admin))
	testifyassert.False(t, h.isSystemAdminRequest(ctx, genPlainUser()))
	testifyassert.False(t, h.isSystemAdminRequest(ctx, nil))
}
