/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

func rule(resources, granted []string, verbs ...v1.RoleVerb) v1.PolicyRule {
	return v1.PolicyRule{Resources: resources, GrantedUsers: granted, Verbs: verbs}
}

func roleOf(rules ...v1.PolicyRule) *v1.Role { return &v1.Role{Rules: rules} }

// HoldsNoAccess reads the rules, not the names: a role grants no access exactly when every rule it
// has is get on the user resource, to the owner. Anything else, including a rule that leaves a field
// empty (which means everything) or names one more resource or verb, is access.
func TestHoldsNoAccess(t *testing.T) {
	user := func(managed ...string) *v1.User {
		u := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u"}}
		commonuser.AddManagedWorkspace(u, managed...)
		return u
	}
	holding := func(roles ...v1.UserRole) *v1.User {
		u := user()
		u.Spec.Roles = roles
		return u
	}
	selfRead := authtest.ChartRole(t, v1.SelfReadRole)
	own := rule([]string{"user"}, []string{"owner"}, v1.GetVerb)
	for _, tt := range []struct {
		name  string
		user  *v1.User
		roles []*v1.Role
		want  bool
	}{
		{"no user", nil, nil, true},
		{"no roles at all", user(), nil, true},
		{"a role that could not be found is no role", user(), []*v1.Role{nil}, true},
		{"the shipped self-read role", user(), []*v1.Role{selfRead}, true},
		{"the shipped default role", user(), []*v1.Role{authtest.ChartRole(t, v1.DefaultRole)}, false},
		{"the shipped system-admin role", user(), []*v1.Role{authtest.ChartRole(t, v1.SystemAdminRole)}, false},
		{"the shipped system-admin-readonly role", user(), []*v1.Role{authtest.ChartRole(t, v1.SystemAdminReadonlyRole)}, false},
		{"the shipped workspace-admin role", user(), []*v1.Role{authtest.ChartRole(t, v1.WorkspaceAdminRole)}, false},
		{"self-read together with default", user(), []*v1.Role{selfRead, authtest.ChartRole(t, v1.DefaultRole)}, false},
		{"a role with no rules", user(), []*v1.Role{roleOf()}, true},
		{"a role of the same shape under another name", user(), []*v1.Role{roleOf(own)}, true},
		{"two rules of that shape", user(), []*v1.Role{roleOf(own, own)}, true},
		{"one more verb", user(), []*v1.Role{roleOf(rule([]string{"user"}, []string{"owner"}, v1.GetVerb, v1.ListVerb))}, false},
		{"list instead of get", user(), []*v1.Role{roleOf(rule([]string{"user"}, []string{"owner"}, v1.ListVerb))}, false},
		{"every verb", user(), []*v1.Role{roleOf(rule([]string{"user"}, []string{"owner"}, v1.AllVerb))}, false},
		{"one more resource", user(), []*v1.Role{roleOf(rule([]string{"user", "workload"}, []string{"owner"}, v1.GetVerb))}, false},
		{"every resource", user(), []*v1.Role{roleOf(rule([]string{"*"}, []string{"owner"}, v1.GetVerb))}, false},
		{"another sub-resource of user", user(), []*v1.Role{roleOf(rule([]string{"user/identity"}, []string{"owner"}, v1.GetVerb))}, false},
		{"every user, not the owner", user(), []*v1.Role{roleOf(rule([]string{"user"}, []string{"*"}, v1.GetVerb))}, false},
		{"workspace users, not the owner", user(), []*v1.Role{roleOf(rule([]string{"user"}, []string{"workspace-user"}, v1.GetVerb))}, false},
		{"the owner and one more", user(), []*v1.Role{roleOf(rule([]string{"user"}, []string{"owner", "workspace-user"}, v1.GetVerb))}, false},
		{"no granted users means everyone", user(), []*v1.Role{roleOf(rule([]string{"user"}, nil, v1.GetVerb))}, false},
		{"no resources", user(), []*v1.Role{roleOf(rule(nil, []string{"owner"}, v1.GetVerb))}, false},
		{"no verbs", user(), []*v1.Role{roleOf(rule([]string{"user"}, []string{"owner"}))}, false},
		{"one good rule and one that grants more", user(), []*v1.Role{roleOf(own, rule([]string{"workload"}, []string{"owner"}, v1.GetVerb))}, false},
		{"managing a workspace is access", user("ws-a"), []*v1.Role{selfRead}, false},
		// A system administrator holds access by being one, whatever the Role objects say: the platform Role
		// missing or unreadable must not lock out the people who would have to put it right.
		{"a system administrator whose Role could not be found", holding(v1.SystemAdminRole), nil, false},
		{"a system administrator whose Role is unreadable", holding(v1.SystemAdminRole), []*v1.Role{nil}, false},
		{"a system administrator with the minimal role next to it", holding(v1.SystemAdminRole, v1.SelfReadRole), []*v1.Role{selfRead}, false},
		{"a read-only administrator whose Role could not be found", holding(v1.SystemAdminReadonlyRole), nil, false},
		{"a read-only administrator with only the minimal role found", holding(v1.SystemAdminReadonlyRole, v1.SelfReadRole), []*v1.Role{selfRead}, false},
		// The exemption is for the platform roles alone: another role that cannot be found is no access.
		{"the default role could not be found", holding(v1.DefaultRole), nil, true},
		{"a role nobody installed", holding("a-role-nobody-installed"), nil, true},
		{"the minimal role alone", holding(v1.SelfReadRole), []*v1.Role{selfRead}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, HoldsNoAccess(tt.user, tt.roles))
		})
	}
}

// accessEnv serves a few routes behind AuthenticateRequest, with the roles the chart ships, and
// returns the status of a request made as a user. A handler that is reached answers 200.
type accessEnv struct {
	t      *testing.T
	engine *gin.Engine
	cli    client.Client
}

func newAccessEnv(t *testing.T, users ...*v1.User) *accessEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	objs := authtest.ChartRoles(t)
	for _, u := range users {
		objs = append(objs, u)
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	old := defaultTokenInstance
	defaultTokenInstance = &defaultToken{Client: cli}
	commonconfig.SetValue("crypto.enable", "false")
	commonconfig.SetValue("user.token.expire", "-1")
	t.Cleanup(func() {
		defaultTokenInstance = old
		commonconfig.SetValue("crypto.enable", "")
		commonconfig.SetValue("user.token.expire", "")
	})

	e := gin.New()
	e.Use(gin.Recovery())
	api := e.Group("/api/v1", func(c *gin.Context) {
		if err := AuthenticateRequest(c); err != nil {
			abortAs(c, err)
			return
		}
		c.Next()
	})
	reached := func(c *gin.Context) { c.String(http.StatusOK, "reached") }
	api.GET("/users/:name", reached)
	api.GET("/users/self/workspaces", reached)
	api.POST("/logout", reached)
	api.GET("/users", reached)
	api.GET("/users/:name/settings", reached)
	api.PUT("/users/:name/settings", reached)
	api.GET("/clusters", reached)
	api.GET("/workloads", reached)
	api.POST("/workloads", reached)
	api.POST("/users/:name", reached) // the same path as the allowed GET, another method
	return &accessEnv{t: t, engine: e, cli: cli}
}

func (e *accessEnv) call(user *v1.User, method, path string) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+generateTestToken(e.t, user.Name, user.Name))
	w := httptest.NewRecorder()
	e.engine.ServeHTTP(w, req)
	return w
}

// abortAs answers as the apiserver does: the status of the error, with its code and message.
func abortAs(c *gin.Context, err error) {
	status, code := http.StatusInternalServerError, ""
	if se, ok := err.(interface{ Status() metav1.Status }); ok {
		status, code = int(se.Status().Code), string(se.Status().Reason)
	}
	c.AbortWithStatusJSON(status, gin.H{"errorCode": code, "errorMessage": err.Error()})
}

func accessUser(name string, roles ...v1.UserRole) *v1.User {
	return &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: roles},
	}
}

// A user whose roles grant nothing but reading their own record is served that, the workspace lists
// it names, and sign-out; every other route answers 403 before its handler runs, whether or not the
// handler would have checked. A user who holds access is served what the roles grant, as ever.
func TestAuthenticateRequestRefusesAUserWhoHoldsNoAccess(t *testing.T) {
	selfRead := accessUser("self-read-user", v1.SelfReadRole)
	plain := accessUser("plain-user", v1.DefaultRole)
	admin := accessUser("admin-user", v1.SystemAdminRole)
	readonly := accessUser("readonly-user", v1.SystemAdminReadonlyRole)
	norole := accessUser("no-role-user")
	ghostRole := accessUser("ghost-role-user", "a-role-nobody-installed")
	both := accessUser("both-user", v1.SelfReadRole, v1.DefaultRole)
	manager := accessUser("manager-user", v1.SelfReadRole)
	commonuser.AddManagedWorkspace(manager, "ws-a")
	env := newAccessEnv(t, selfRead, plain, admin, readonly, norole, ghostRole, both, manager)

	const open, shut = http.StatusOK, http.StatusForbidden
	for _, tt := range []struct {
		name         string
		user         *v1.User
		method, path string
		want         int
	}{
		// What a user who holds no access is served.
		{"self-read: its own record", selfRead, http.MethodGet, "/api/v1/users/self", open},
		{"self-read: a record by id", selfRead, http.MethodGet, "/api/v1/users/" + selfRead.Name, open},
		{"self-read: the workspace lists", selfRead, http.MethodGet, "/api/v1/users/self/workspaces", open},
		{"self-read: sign out", selfRead, http.MethodPost, "/api/v1/logout", open},
		// Everything else is refused, whatever the handler would have done.
		{"self-read: the user list", selfRead, http.MethodGet, "/api/v1/users", shut},
		{"self-read: its settings", selfRead, http.MethodGet, "/api/v1/users/self/settings", shut},
		{"self-read: changing its settings", selfRead, http.MethodPut, "/api/v1/users/self/settings", shut},
		{"self-read: the clusters", selfRead, http.MethodGet, "/api/v1/clusters", shut},
		{"self-read: the workloads", selfRead, http.MethodGet, "/api/v1/workloads", shut},
		{"self-read: creating a workload", selfRead, http.MethodPost, "/api/v1/workloads", shut},
		{"self-read: another method on the path of an allowed route", selfRead, http.MethodPost, "/api/v1/users/self", shut},
		// The router redirects a trailing slash to the route; the redirected request meets the gate itself.
		{"self-read: a trailing slash is a redirect to the route, not a way around the gate", selfRead, http.MethodGet, "/api/v1/users/self/workspaces/", http.StatusMovedPermanently},
		// A route the table does not have is not served to it either (the router answers 404, and the gate is
		// never reached): this pins that the allowed shapes are matched by route, not by a prefix.
		{"self-read: an unknown route", selfRead, http.MethodGet, "/api/v1/users/self/anything", http.StatusNotFound},
		// A user without a usable role holds nothing.
		{"no role: the clusters", norole, http.MethodGet, "/api/v1/clusters", shut},
		{"a role nobody installed: nothing", ghostRole, http.MethodGet, "/api/v1/clusters", shut},
		// A user who holds access.
		{"default: the clusters", plain, http.MethodGet, "/api/v1/clusters", open},
		{"default: creating a workload", plain, http.MethodPost, "/api/v1/workloads", open},
		{"default: its own record", plain, http.MethodGet, "/api/v1/users/self", open},
		{"admin: the user list", admin, http.MethodGet, "/api/v1/users", open},
		{"readonly admin: the user list", readonly, http.MethodGet, "/api/v1/users", open},
		{"self-read with default: the clusters", both, http.MethodGet, "/api/v1/clusters", open},
		{"self-read that manages a workspace: the workloads", manager, http.MethodGet, "/api/v1/workloads", open},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := env.call(tt.user, tt.method, tt.path)
			require.Equal(t, tt.want, w.Code, w.Body.String())
			if tt.want == shut {
				require.Contains(t, w.Body.String(), ErrNoAccess, "refused by the gate, not by something else")
				require.Contains(t, w.Body.String(), commonerrors.NoAccess, "with the error code a client tells it by")
				require.NotContains(t, w.Body.String(), "reached", "the handler must not run")
			}
		})
	}
}

// The platform Roles of a deployment missing (the objects installed in the wrong order, say) must not lock
// the system administrators out of every route: they are the people who have to put it right, and the check
// that lets them through the administrator routes (AuthorizeSystemAdmin) reads the role name alone. An
// ordinary user whose Role is missing holds no access, as ever.
func TestAuthenticateRequestServesAdministratorsWhoseRoleIsMissing(t *testing.T) {
	admin := accessUser("admin-user", v1.SystemAdminRole)
	readonly := accessUser("readonly-user", v1.SystemAdminReadonlyRole)
	plain := accessUser("plain-user", v1.DefaultRole)
	env := newAccessEnv(t, admin, readonly, plain)
	for _, name := range []v1.UserRole{v1.SystemAdminRole, v1.SystemAdminReadonlyRole, v1.DefaultRole} {
		require.NoError(t, env.cli.Delete(t.Context(), &v1.Role{ObjectMeta: metav1.ObjectMeta{Name: string(name)}}))
	}

	for _, tt := range []struct {
		name string
		user *v1.User
		want int
	}{
		{"a system administrator", admin, http.StatusOK},
		{"a read-only administrator", readonly, http.StatusOK},
		{"an ordinary user", plain, http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := env.call(tt.user, http.MethodGet, "/api/v1/users")
			require.Equal(t, tt.want, w.Code, w.Body.String())
			if tt.want == http.StatusForbidden {
				require.Contains(t, w.Body.String(), commonerrors.NoAccess)
			}
		})
	}
}

// In external mode the user the request sees holds the base roles whatever the record stores, so what a
// new identity may do follows users.base_roles: the minimal role holds no access, the default does.
func TestAuthenticateRequestFollowsTheBaseRolesInExternalMode(t *testing.T) {
	for _, tt := range []struct {
		name  string
		base  any
		path  string
		stats int
	}{
		{"unset: a new identity holds the default role", nil, "/api/v1/clusters", http.StatusOK},
		{"minimal: the clusters are refused", []string{"self-read"}, "/api/v1/clusters", http.StatusForbidden},
		{"minimal: its own record is served", []string{"self-read"}, "/api/v1/users/self", http.StatusOK},
		{"base roles that include the default role", []string{"self-read", "default"}, "/api/v1/clusters", http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set("users.base_roles", tt.base)
			t.Cleanup(func() { viper.Set("users.base_roles", nil) })
			e := newExtEnv(t, nil, false)
			for _, r := range authtest.ChartRoles(t) {
				require.NoError(t, e.cli.Create(t.Context(), r.(*v1.Role)))
			}
			e.revUsers["tok"] = map[string]any{"sub": "newcomer", "username": "newcomer", "aud": extAudience}

			gin.SetMode(gin.TestMode)
			engine := gin.New()
			api := engine.Group("/api/v1", func(c *gin.Context) {
				if err := AuthenticateRequest(c); err != nil {
					abortAs(c, err)
					return
				}
				c.Next()
			})
			api.GET("/clusters", func(c *gin.Context) { c.String(http.StatusOK, "reached") })
			api.GET("/users/:name", func(c *gin.Context) { c.String(http.StatusOK, "reached") })

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Authorization", "Bearer tok")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			require.Equal(t, tt.stats, w.Code, w.Body.String())

			// The record gained no role from this: the base roles are never stored by the apiserver.
			u := &v1.User{}
			require.NoError(t, e.cli.Get(t.Context(), client.ObjectKey{Name: "newcomer"}, u))
			require.Empty(t, u.Spec.Roles)
		})
	}
}

// The exported route set is exactly the three routes the design names.
func TestNoAccessRoutesAreExactlyTheOwnRecordRoutes(t *testing.T) {
	require.True(t, IsNoAccessRoute(http.MethodGet, "/api/v1/users/:name"))
	require.True(t, IsNoAccessRoute(http.MethodGet, "/api/v1/users/self/workspaces"))
	require.True(t, IsNoAccessRoute(http.MethodPost, "/api/v1/logout"))
	require.Len(t, noAccessRoutes, 3)
	for _, not := range [][2]string{
		{http.MethodPut, "/api/v1/users/:name"},
		{http.MethodDelete, "/api/v1/users/:name"},
		{http.MethodGet, "/api/v1/users"},
		{http.MethodGet, "/api/v1/users/:name/settings"},
		{http.MethodGet, ""},
		{http.MethodGet, "/api/v1/users/self"},
	} {
		require.False(t, IsNoAccessRoute(not[0], not[1]), "%s %s", not[0], not[1])
	}
}
