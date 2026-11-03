/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cdhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// cdTestUser returns a User with the given id and roles.
func cdTestUser(id string, roles ...v1.UserRole) *v1.User {
	return &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: id},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: roles},
	}
}

// serveCD runs one request through the CD route guard and reports the status
// and whether the request reached the route handler.
func serveCD(h *Handler, method, userId string) (int, bool) {
	reached := false
	e := gin.New()
	e.Use(func(c *gin.Context) { c.Set(common.UserId, userId) }, h.authorizeSystemAdmin())
	e.Handle(method, "/api/v1/cd/deployments", func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})
	rsp := httptest.NewRecorder()
	e.ServeHTTP(rsp, httptest.NewRequest(method, "/api/v1/cd/deployments", nil))
	return rsp.Code, reached
}

func TestCDRoutesRequireSystemAdmin(t *testing.T) {
	h := &Handler{accessController: &authority.AccessController{Client: createFakeCtrlClient(
		cdTestUser("admin", v1.SystemAdminRole),
		cdTestUser("readonly", v1.SystemAdminReadonlyRole),
		cdTestUser("member", v1.DefaultRole),
	)}}

	cases := []struct {
		user    string
		method  string
		allowed bool
	}{
		{"admin", http.MethodGet, true},
		{"admin", http.MethodPost, true},
		{"readonly", http.MethodGet, true},
		{"readonly", http.MethodPost, false},
		{"member", http.MethodGet, false},
		{"member", http.MethodPost, false},
		{"", http.MethodPost, false},
		{"unknown", http.MethodPost, false},
	}
	for _, tc := range cases {
		code, reached := serveCD(h, tc.method, tc.user)
		assert.Equal(t, tc.allowed, reached, "%s %s reached the handler", tc.user, tc.method)
		if !tc.allowed {
			assert.NotEqual(t, http.StatusOK, code, "%s %s status", tc.user, tc.method)
		}
	}
}
