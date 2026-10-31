/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apimetrics "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// ErrNoAccess is the answer to a user whose roles grant nothing but reading the user's own record
// when the request is for anything else. A client tells it from other refusals by this text.
const ErrNoAccess = "The user has no access yet. Ask an administrator to grant it"

// userResource is the resource kind the user's own record is read as (v1.UserKind, lower case).
const userResource = "user"

// noAccessRoutes are the routes a user who holds no access (see HoldsNoAccess) is served: reading
// the user's own record, and the lists of workspaces that record names, and signing out. They are
// identified by the route the request matched, not by its URL, so a path that merely looks like
// one of them is not served. Whatever a handler behind them answers is for the user's own data.
//
// Everything else is refused before its handler runs. That is the rule, not a list of the handlers
// that check: a handler that forgets to check, or is added without one, is still closed to such a
// user. The handlers' own checks stay, and are what a user who holds access is held to.
func noAccessRoutes() map[string]bool {
	root := "/" + common.PrimusRouterCustomRootPath
	return map[string]bool{
		http.MethodGet + " " + root + "/users/:" + common.Name: true,
		http.MethodGet + " " + root + "/users/self/workspaces": true,
		http.MethodPost + " " + root + "/logout":               true,
	}
}

// IsNoAccessRoute reports whether method and the route the request matched (the pattern, such as
// "/api/v1/users/:name", as gin's FullPath gives it) are served to a user who holds no access.
func IsNoAccessRoute(method, fullPath string) bool {
	return noAccessRoutes()[method+" "+fullPath]
}

// HoldsNoAccess reports whether the roles grant a user nothing: no rule among them lets the user do
// anything but read (get) the user's own user record. A user with no roles at all, or whose roles
// cannot be found, holds no access either: closed is the safe answer for the one check that has to
// hold on every route. The user's managed workspaces count as access, as they do in Authorize.
//
// It reads the rules, not the names of the roles, so it needs no knowledge of which role a
// deployment gives a new user, and a role of the same shape is judged the same.
func HoldsNoAccess(user *v1.User, roles []*v1.Role) bool {
	if user == nil {
		return true
	}
	if len(commonuser.GetManagedWorkspace(user)) > 0 {
		return false
	}
	for _, role := range roles {
		if role == nil {
			continue
		}
		for i := range role.Rules {
			if !isOwnRecordReadRule(&role.Rules[i]) {
				return false
			}
		}
	}
	return true
}

// isOwnRecordReadRule reports whether the rule grants nothing but get on the user resource to the
// owner of the record. An empty field means "everything" (an empty grantedUsers matches every
// user), so only a rule that names what it grants, and grants it to the owner, is within it.
func isOwnRecordReadRule(r *v1.PolicyRule) bool {
	if len(r.Resources) == 0 || len(r.GrantedUsers) == 0 || len(r.Verbs) == 0 {
		return false
	}
	for _, res := range r.Resources {
		if res != userResource {
			return false
		}
	}
	for _, g := range r.GrantedUsers {
		if g != GrantedOwner {
			return false
		}
	}
	for _, verb := range r.Verbs {
		if verb != v1.GetVerb {
			return false
		}
	}
	return true
}

// accessClient returns the client the roles of the authenticated user are read through: the one the
// credential was authenticated with. It is nil when none is initialized.
func accessClient() client.Client {
	if st := externalState.Load(); st != nil && st.client != nil {
		return st.client
	}
	if t := defaultTokenInstance; t != nil && t.Client != nil {
		return t.Client
	}
	if t := apiKeyTokenInstance; t != nil && t.Client != nil {
		return t.Client
	}
	if t := ssoInstance; t != nil && t.Client != nil {
		return t.Client
	}
	return nil
}

// AuthenticateRequest is what every route that serves a user starts with: it authenticates the
// request (ParseToken) and then refuses a user who holds no access any route but the ones such a
// user is served (RequireAccess). The user it authenticated is in the context when it returns nil.
//
// It is the one place the rule that a user who holds no access is served nothing but the user's
// own record is enforced, so it does not depend on each handler checking, and a route that is added
// is closed to such a user without anyone having to remember. Every caller of ParseToken that
// serves a user calls this instead; the route-table test fails for a route that does not.
func AuthenticateRequest(c *gin.Context) error {
	if err := ParseToken(c); err != nil {
		return err
	}
	return RequireAccess(c)
}

// RequireAccess refuses the request of an authenticated user who holds no access, unless it is one
// of the routes such a user is served (noAccessRoutes). It runs after the credential is accepted,
// so the user, and in external mode the request's overlay, are known.
func RequireAccess(c *gin.Context) error {
	if IsNoAccessRoute(c.Request.Method, c.FullPath()) {
		return nil
	}
	cli := accessClient()
	if cli == nil {
		klog.Error("no client to read the roles of the authenticated user with")
		return commonerrors.NewInternalError("failed to check the user's access")
	}
	ctx := c.Request.Context()
	ac := &AccessController{Client: cli}
	user, err := ac.GetRequestUser(ctx, c.GetString(common.UserId))
	if err != nil {
		return err
	}
	if !HoldsNoAccess(user, ac.GetRoles(ctx, user)) {
		return nil
	}
	apimetrics.AuthFailuresTotal.WithLabelValues("no_access").Inc()
	klog.Warningf("refused %s %s: user %s holds no access", c.Request.Method, c.FullPath(), user.Name)
	return commonerrors.NewForbidden(ErrNoAccess)
}
