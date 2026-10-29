/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// extCacheTTL is the introspection cache_ttl newExtEnvWith configures.
const extCacheTTL = 50 * time.Millisecond

// credential is one way to present the same opaque credential to the apiserver.
type credential struct {
	name string
	// call authenticates a request carrying the credential, as the auth middleware does.
	call func(t *testing.T, token string) (*gin.Context, error)
}

var extCredentials = []credential{
	{"session cookie", func(t *testing.T, token string) (*gin.Context, error) {
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/workloads", nil)
		c.Request.AddCookie(&http.Cookie{Name: CookieToken, Value: token})
		return c, ParseToken(c)
	}},
	{"api key", func(t *testing.T, token string) (*gin.Context, error) {
		return extParse(t, http.MethodGet, "/api/v1/workloads", token)
	}},
}

// whatTheRequestSees is what an authorization check made on this request finds: the answer
// of the access controller and of the user as the handlers fetch it.
type whatTheRequestSees struct{ authorizes, isAdmin, isReadonly bool }

func (e *extEnv) see(c *gin.Context) whatTheRequestSees {
	e.t.Helper()
	ac := &AccessController{Client: e.cli}
	ctx, id := c.Request.Context(), c.GetString(common.UserId)
	user, err := ac.GetRequestUser(ctx, id)
	require.NoError(e.t, err)
	return whatTheRequestSees{
		authorizes: ac.AuthorizeSystemAdmin(AccessInput{Context: ctx, UserId: id}, false) == nil,
		isAdmin:    user.IsSystemAdmin(),
		isReadonly: user.IsSystemAdminReadonly(),
	}
}

func (e *extEnv) storeUser(roles ...v1.UserRole) {
	e.t.Helper()
	require.NoError(e.t, e.cli.Create(context.Background(), &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: extUser, Annotations: map[string]string{v1.UserNameAnnotation: "alice"}},
		Spec:       v1.UserSpec{Type: v1.SSOUserType, Roles: roles},
	}))
}

// E11: platform roles come only from the identity. Revoking one upstream removes it from the
// apiserver within the introspection cache_ttl although the user object still lists it, and
// restoring it brings it back. The same for a session cookie and an API key.
func TestPlatformRoleRevocationTakesEffectWithinCacheTTL(t *testing.T) {
	admin := whatTheRequestSees{authorizes: true, isAdmin: true}
	none := whatTheRequestSees{}
	for _, cred := range extCredentials {
		t.Run(cred.name, func(t *testing.T) {
			e := newExtEnv(t, adminMapping, false)
			// The state of a real deployment on the day the identity provider takes over: the
			// user object already lists system-admin, from before the identity existed.
			e.storeUser(v1.SystemAdminRole)
			const token = "ak-alice"
			inGroup := map[string]any{"sub": extUser, "username": "alice", "groups": []string{"platform-admins"}}
			e.revUsers[token] = inGroup

			c, err := cred.call(t, token)
			require.NoError(t, err)
			require.Equal(t, admin, e.see(c), "before the revocation")

			// Revoked upstream: the answer is still active, the group is gone.
			e.revUsers[token] = map[string]any{"sub": extUser, "username": "alice", "groups": []string{}}
			time.Sleep(3 * extCacheTTL) // past cache_ttl
			c, err = cred.call(t, token)
			require.NoError(t, err, "the credential is still valid: only the role was revoked")
			require.Equal(t, none, e.see(c), "revoked: the stored role no longer counts")

			// Restored upstream: the positive control, so the denial above cannot be a broken fixture.
			e.revUsers[token] = inGroup
			time.Sleep(3 * extCacheTTL)
			c, err = cred.call(t, token)
			require.NoError(t, err)
			require.Equal(t, admin, e.see(c), "restored")

			// Nothing was written to the user object by any of this.
			require.Equal(t, []v1.UserRole{v1.SystemAdminRole}, e.user(extUser).Spec.Roles)
		})
	}
}

// Disabling the identity upstream refuses the credential outright.
func TestDisabledIdentityIsRefusedWithinCacheTTL(t *testing.T) {
	for _, cred := range extCredentials {
		t.Run(cred.name, func(t *testing.T) {
			e := newExtEnv(t, adminMapping, false)
			e.storeUser(v1.SystemAdminRole)
			const token = "ak-alice"
			e.revUsers[token] = map[string]any{"sub": extUser, "username": "alice", "groups": []string{"platform-admins"}}
			_, err := cred.call(t, token)
			require.NoError(t, err)

			delete(e.revUsers, token) // the endpoint now answers active:false
			time.Sleep(3 * extCacheTTL)
			c, err := cred.call(t, token)
			require.True(t, apierrors.IsUnauthorized(err), "%v", err)
			require.Empty(t, c.GetString(common.UserId))
		})
	}
}

// Only the platform roles are the identity's alone; any other role is still a union, and a
// read-only administrator granted by group is one.
func TestOnlyPlatformRolesAreTakenFromTheIdentity(t *testing.T) {
	e := newExtEnv(t, []map[string]any{
		{"group": "platform-admins", "roles": []string{"system-admin"}},
		{"group": "auditors", "roles": []string{"system-admin-readonly"}},
	}, false)
	e.storeUser("data-scientist", v1.SystemAdminReadonlyRole, v1.DefaultRole)
	e.revUsers["ak-alice"] = map[string]any{"sub": extUser, "username": "alice", "groups": []string{}}
	c, err := extCredentials[1].call(t, "ak-alice")
	require.NoError(t, err)
	require.Equal(t, whatTheRequestSees{}, e.see(c), "the stored read-only role is revoked")
	user, err := (&AccessController{Client: e.cli}).GetRequestUser(c.Request.Context(), extUser)
	require.NoError(t, err)
	require.ElementsMatch(t, []v1.UserRole{"data-scientist", v1.DefaultRole}, user.Spec.Roles)

	e.revUsers["ak-bob"] = map[string]any{"sub": extUser, "username": "alice", "groups": []string{"auditors"}}
	c, err = extCredentials[1].call(t, "ak-bob")
	require.NoError(t, err)
	require.Equal(t, whatTheRequestSees{isReadonly: true}, e.see(c), "granted by the group")
}

// There is no local-account exception: a request the chain handed to the built-in mode was not
// vouched for by an identity provider, so the platform roles on its user object do not count,
// also while the identity provider is down. (Whether a chain should keep the built-in entry at
// all is for the deployment; this is only what such a request holds.)
func TestBuiltinEntryDoesNotKeepStoredPlatformRoles(t *testing.T) {
	e := newExtEnv(t, adminMapping, true)
	defer setupDefaultToken(t)()
	require.NoError(t, e.cli.Create(context.Background(), &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "local-admin"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
	}))
	call := func() *gin.Context {
		c, err := extParse(t, http.MethodGet, "/api/v1/users/self", generateTestToken(t, "local-admin", "admin"))
		require.NoError(t, err, "the credential itself is still accepted")
		require.Equal(t, "local-admin", c.GetString(common.UserId))
		return c
	}
	require.Equal(t, whatTheRequestSees{}, e.see(call()))
	e.revMode.Store("503")
	require.Equal(t, whatTheRequestSees{}, e.see(call()), "an outage of the identity provider does not bring the stored role back")
	// The record is untouched.
	require.Equal(t, []v1.UserRole{v1.SystemAdminRole}, e.user("local-admin").Spec.Roles)
}

// Builtin mode has no chain and no overlay: the stored roles are the roles, as before.
func TestBuiltinModeKeepsStoredPlatformRoles(t *testing.T) {
	viper.Set("auth.mode", nil)
	require.NoError(t, InitExternalAuth(nil))
	defer setupDefaultToken(t)()
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "local-admin"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
	}).Build()
	c, err := extParse(t, http.MethodGet, "/api/v1/users/self", generateTestToken(t, "local-admin", "admin"))
	require.NoError(t, err)
	ac := &AccessController{Client: cli}
	require.NoError(t, ac.AuthorizeSystemAdmin(AccessInput{Context: c.Request.Context(), UserId: "local-admin"}, false))
}

// What GetRequestUser returns is the user as the request sees it, and never the record.
func TestGetRequestUserAppliesTheOverlay(t *testing.T) {
	e := newExtEnv(t, adminMapping, false)
	ac := &AccessController{Client: e.cli}
	e.revUsers["ak-alice"] = map[string]any{"sub": extUser, "username": "alice", "groups": []string{"platform-admins", "team-a"}}
	c, err := extParse(t, http.MethodGet, "/api/v1/workloads", "ak-alice")
	require.NoError(t, err)

	user, err := ac.GetRequestUser(c.Request.Context(), extUser)
	require.NoError(t, err)
	require.True(t, user.IsSystemAdmin(), "the mapped role is part of the user the handlers see")
	require.Contains(t, user.Spec.Resources[common.UserWorkspaces], "ws-a")

	// Not a request context: no overlay, the record as it is stored.
	plain, err := ac.GetRequestUser(context.Background(), extUser)
	require.NoError(t, err)
	require.False(t, plain.IsSystemAdmin())

	// The cluster still holds only what was stored.
	stored := e.user(extUser)
	require.Empty(t, stored.Spec.Roles)
	require.Empty(t, stored.Spec.Resources)
}

// A user that no identity provider vouched for (an SSH session, which is authenticated by public key)
// follows the rule of a built-in credential: in external mode the platform roles stored on the user
// object do not count, and the rest of the record does. Builtin mode keeps the stored roles.
func TestWithoutIdentityDropsStoredPlatformRoles(t *testing.T) {
	e := newExtEnv(t, adminMapping, false)
	e.storeUser(v1.SystemAdminRole)
	ac := &AccessController{Client: e.cli}
	isAdmin := func(ctx context.Context) bool {
		user, err := ac.GetRequestUser(ctx, extUser)
		require.NoError(t, err)
		return user.IsSystemAdmin() && ac.AuthorizeSystemAdmin(AccessInput{Context: ctx, UserId: extUser}, false) == nil
	}
	// Positive control: the record as stored is an administrator, so the refusal below is not a fixture
	// that never held the role.
	require.True(t, isAdmin(context.Background()))

	ctx := WithoutIdentity(context.Background(), extUser)
	require.False(t, isAdmin(ctx), "the stored platform role does not count")
	user, err := ac.GetRequestUser(ctx, extUser)
	require.NoError(t, err)
	require.Equal(t, []v1.UserRole{v1.DefaultRole}, user.Spec.Roles, "an ordinary user")
	require.Equal(t, []v1.UserRole{v1.SystemAdminRole}, e.user(extUser).Spec.Roles, "the record is untouched")

	// Another user's record is not touched by the overlay of this one.
	other, err := ac.GetRequestUser(WithoutIdentity(context.Background(), "someone-else"), extUser)
	require.NoError(t, err)
	require.True(t, other.IsSystemAdmin(), "an overlay is for the user it names")

	// Builtin mode: no chain, the context is returned as it is and the stored roles are the roles.
	viper.Set("auth.mode", nil)
	require.NoError(t, InitExternalAuth(nil))
	plain := context.WithValue(context.Background(), struct{}{}, "marker")
	require.Equal(t, plain, WithoutIdentity(plain, extUser))
	require.True(t, isAdmin(WithoutIdentity(context.Background(), extUser)))
}

// An operator who relied on a local administrator is told at start-up that stored platform roles no
// longer count, and, when the chain has a builtin entry, that it keeps the credentials but not the roles.
func TestExternalModeWarnsThatStoredPlatformRolesNoLongerCount(t *testing.T) {
	for _, builtin := range []bool{false, true} {
		name := "chain without builtin"
		if builtin {
			name = "chain with builtin"
		}
		t.Run(name, func(t *testing.T) {
			e := newExtEnv(t, adminMapping, builtin)
			out := captureKlog(t, func() { require.NoError(t, InitExternalAuth(e.cli)) })
			require.Contains(t, out, "no longer count")
			require.Contains(t, out, "system-admin, system-admin-readonly")
			require.Equal(t, builtin, strings.Contains(out, "builtin authenticator"), out)
		})
	}

	// Builtin mode changes nothing, so it warns of nothing.
	viper.Set("auth.mode", nil)
	out := captureKlog(t, func() { require.NoError(t, InitExternalAuth(nil)) })
	require.NotContains(t, out, "no longer count")
}
