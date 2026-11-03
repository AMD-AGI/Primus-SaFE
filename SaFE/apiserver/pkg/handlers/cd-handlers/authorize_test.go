/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cdhandlers

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const restrictedMessage = "The user is restricted"

// cdTestUser returns a User named by name with the given roles and a password of "pw".
func cdTestUser(name string, restricted bool, roles ...v1.UserRole) *v1.User {
	id := commonuser.GenerateUserIdByName(name)
	u := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: id, Labels: map[string]string{v1.UserIdLabel: id}, Annotations: map[string]string{v1.UserNameAnnotation: name}},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Password: stringutil.Base64Encode("pw")},
	}
	if restricted {
		u.Spec.RestrictedType = v1.UserFrozen
	}
	return u
}

// cdTestUsers returns the users the CD authorization tests sign in as.
func cdTestUsers() []*v1.User {
	return []*v1.User{
		cdTestUser("admin", false, v1.SystemAdminRole),
		cdTestUser("readonly", false, v1.SystemAdminReadonlyRole),
		cdTestUser("member", false, v1.DefaultRole),
		cdTestUser("frozen-admin", true, v1.SystemAdminRole),
	}
}

// serveGuard runs one request through the CD route guard alone and reports the
// status, the body and whether the request reached the route handler.
func serveGuard(h *Handler, method, userId string) (int, string, bool) {
	reached := false
	e := gin.New()
	e.Use(func(c *gin.Context) { c.Set(common.UserId, userId) }, h.authorizeSystemAdmin())
	e.Handle(method, "/api/v1/cd/deployments", func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})
	rsp := httptest.NewRecorder()
	e.ServeHTTP(rsp, httptest.NewRequest(method, "/api/v1/cd/deployments", nil))
	return rsp.Code, rsp.Body.String(), reached
}

func TestCDGuardRequiresSystemAdmin(t *testing.T) {
	var objs []ctrlclient.Object
	for _, u := range cdTestUsers() {
		objs = append(objs, u)
	}
	h := &Handler{accessController: &authority.AccessController{Client: createFakeCtrlClient(objs...)}}
	id := commonuser.GenerateUserIdByName

	cases := []struct {
		user   string
		method string
		code   int
		body   string
	}{
		{id("admin"), http.MethodGet, http.StatusOK, ""},
		{id("admin"), http.MethodPost, http.StatusOK, ""},
		{id("readonly"), http.MethodGet, http.StatusOK, ""},
		{id("readonly"), http.MethodPost, http.StatusForbidden, authority.SystemAdminRequired},
		{id("member"), http.MethodGet, http.StatusForbidden, authority.SystemAdminRequired},
		{id("member"), http.MethodPost, http.StatusForbidden, authority.SystemAdminRequired},
		{id("frozen-admin"), http.MethodGet, http.StatusForbidden, restrictedMessage},
		{id("frozen-admin"), http.MethodPost, http.StatusForbidden, restrictedMessage},
		{"", http.MethodPost, http.StatusBadRequest, "the request userId is empty"},
		{"unknown", http.MethodPost, http.StatusUnauthorized, "unknown"},
	}
	for _, tc := range cases {
		code, body, reached := serveGuard(h, tc.method, tc.user)
		assert.Equal(t, tc.code, code, "%s %s status", tc.user, tc.method)
		assert.Equal(t, tc.code == http.StatusOK, reached, "%s %s reached the handler", tc.user, tc.method)
		assert.Contains(t, body, tc.body, "%s %s body", tc.user, tc.method)
	}
}

var cdRouteParam = regexp.MustCompile(`:[A-Za-z]+`)

// serveRoute serves one request to a route registered by InitCDRouters. A
// handler that panics on its nil dependencies was reached and answers 599.
func serveRoute(e *gin.Engine, r gin.RouteInfo, token string) (code int, body string) {
	req := httptest.NewRequest(r.Method, cdRouteParam.ReplaceAllString(r.Path, "x"), strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rsp := httptest.NewRecorder()
	func() {
		defer func() {
			if recover() != nil {
				rsp.Code = 599
			}
		}()
		e.ServeHTTP(rsp, req)
	}()
	return rsp.Code, rsp.Body.String()
}

func TestCDRoutesRequireSystemAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	commonconfig.SetValue("crypto.enable", "false")
	t.Cleanup(func() { commonconfig.SetValue("crypto.enable", "") })

	objs := authtest.ChartRoles(t)
	for _, u := range cdTestUsers() {
		objs = append(objs, u)
	}
	cli := createFakeCtrlClient(objs...)
	tok := authority.NewDefaultToken(cli)
	require.True(t, tok.Client == cli, "the sign-in is bound to another client")

	e := gin.New()
	InitCDRouters(e, &Handler{accessController: &authority.AccessController{Client: cli}})
	routes := e.Routes()
	require.Len(t, routes, 8, "the CD route table changed")

	tokens := map[string]string{"anonymous": ""}
	for _, u := range cdTestUsers() {
		name := u.Annotations[v1.UserNameAnnotation]
		_, rsp, err := tok.Login(t.Context(), authority.TokenInput{Username: name, Password: "pw"})
		require.NoError(t, err)
		tokens[name] = rsp.Token
	}

	for _, r := range routes {
		read := r.Method == http.MethodGet
		refusals := map[string]struct {
			code int
			body string
		}{
			"anonymous":    {http.StatusUnauthorized, ""},
			"member":       {http.StatusForbidden, authority.SystemAdminRequired},
			"frozen-admin": {http.StatusForbidden, restrictedMessage},
		}
		if !read {
			refusals["readonly"] = refusals["member"]
		}
		for user, token := range tokens {
			code, body := serveRoute(e, r, token)
			want, refused := refusals[user]
			if refused {
				assert.Equal(t, want.code, code, "%s %s %s status", user, r.Method, r.Path)
				assert.Contains(t, body, want.body, "%s %s %s body", user, r.Method, r.Path)
				continue
			}
			assert.NotContains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, code,
				"%s %s %s was refused: %s", user, r.Method, r.Path, body)
		}
	}
}
