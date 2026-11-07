/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	apimetrics "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// builtinKeyActors is, in builtin mode, what POST /auth/verify needs to answer the delegated tokens of the
// parties named in auth.key_actors: the oidc authenticators of auth.authenticators, which check the issuer's
// signature and the token's audience. Nothing else uses it: no request is authenticated with it, no user is
// created by it, and the apiserver's own logins and API keys are untouched. nil: not configured, and a
// delegated token is the unknown credential it has always been in builtin mode.
type builtinKeyActors struct {
	chain     *extauth.Chain
	keyActors map[string]struct{}
	client    client.Client
}

var builtinKeyActorState atomic.Pointer[builtinKeyActors]

// initBuiltinKeyActors builds the builtin-mode key actor path when auth.key_actors names at least one client and
// auth.authenticators holds at least one oidc authenticator. Only oidc entries count: an introspection entry would
// be asked about every bearer token this endpoint sees, including the apiserver's own sessions, and a builtin entry
// means nothing here. A configuration that cannot be built leaves the path off (logged), so a mistake in it can
// never stop the builtin mode from starting.
func initBuiltinKeyActors(cli client.Client) {
	builtinKeyActorState.Store(nil)
	keyActors := map[string]struct{}{}
	for _, a := range commonconfig.GetAuthKeyActors() {
		if a = strings.TrimSpace(a); a != "" {
			keyActors[a] = struct{}{}
		}
	}
	if len(keyActors) == 0 {
		return
	}
	cfgs, err := commonconfig.GetAuthenticators()
	if err != nil {
		klog.ErrorS(err, "auth.key_actors is set in builtin mode but auth.authenticators cannot be read; no delegated token is accepted")
		return
	}
	var oidc []commonconfig.AuthenticatorConfig
	for _, c := range cfgs {
		if strings.EqualFold(c.Type, "oidc") {
			oidc = append(oidc, c)
		} else {
			klog.Warningf("auth.mode is builtin: the %q authenticator %q is not used for auth.key_actors (only oidc is)", c.Type, c.Name)
		}
	}
	if len(oidc) == 0 {
		klog.Warning("auth.key_actors is set in builtin mode but auth.authenticators has no oidc authenticator; no delegated token is accepted")
		return
	}
	chain, err := buildChain(oidc)
	if err != nil {
		klog.ErrorS(err, "auth.key_actors is set in builtin mode but its authenticators cannot be built; no delegated token is accepted")
		return
	}
	if cli == nil {
		klog.Error("auth.key_actors is set in builtin mode but there is no client to read users with; no delegated token is accepted")
		return
	}
	builtinKeyActorState.Store(&builtinKeyActors{chain: chain, keyActors: keyActors, client: cli})
	klog.Infof("auth.mode is builtin: %d client(s) may fetch, with a delegated token, the keys of the existing user it acts for (auth.key_actors)", len(keyActors))
}

// verifyBuiltinKeyActor answers, in builtin mode, a verify request whose credential is a bearer token the key actor
// chain recognises. handled is false when it does not (any other credential, or a token no authenticator accepts,
// or one that is not delegated): the caller then runs the builtin path, exactly as before. When handled is true and
// ok is false the refusal has been written. The answer is about the token's subject, an existing user, and nobody
// else: the request has no field that names another user, and no user is created.
func verifyBuiltinKeyActor(c *gin.Context, st *builtinKeyActors, req *VerifyTokenRequest) (verdict *externalVerdict, handled, ok bool) {
	if req.Cookie != "" || req.ApiKey != "" || req.Authorization == "" || ExtractApiKeyFromRequest(req.Authorization) != "" {
		return nil, false, true
	}
	bearer := extractBearerToken(req.Authorization)
	if strings.Count(bearer, ".") != 2 {
		return nil, false, true
	}
	ctx := c.Request.Context()
	id, err := st.chain.Authenticate(ctx, extauth.Credentials{Bearer: bearer, ClientIP: req.ClientIP})
	if err != nil {
		var unavailable *extauth.UnavailableError
		if errors.As(err, &unavailable) {
			// Only a token of a configured issuer gets here (the oidc authenticator looks at the issuer first):
			// the caller is told to retry, never that the credential is bad, and never answered by the builtin path.
			klog.ErrorS(err, "authentication unavailable", "path", c.Request.URL.Path)
			apimetrics.AuthFailuresTotal.WithLabelValues("unavailable").Inc()
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"code": http.StatusServiceUnavailable, "message": "the authentication service is unavailable",
			})
			return nil, true, false
		}
		return nil, false, true
	}
	if !id.Delegated() {
		return nil, false, true
	}
	refuse := func(status int, metric, message string) (*externalVerdict, bool, bool) {
		apimetrics.AuthFailuresTotal.WithLabelValues(metric).Inc()
		c.AbortWithStatusJSON(status, gin.H{"code": status, "message": message})
		return nil, true, false
	}
	if _, listed := st.keyActors[id.ActorClient]; id.ActorClient == "" || !listed {
		klog.Warningf("delegated credential refused: capability %s, actor %s, user %s", extauth.CapabilityPlatformKey, id.Actor, id.ID)
		return refuse(http.StatusForbidden, "delegated_forbidden", "this operation is not available with a delegated token")
	}
	if !extauth.HasScope(id.Scopes, extauth.ScopeAPI) {
		klog.Warningf("delegated credential refused: scope %v does not include %s, actor %s, user %s", id.Scopes, extauth.ScopeAPI, id.Actor, id.ID)
		return refuse(http.StatusForbidden, "delegated_scope", "the token scope does not allow this operation")
	}
	user := &v1.User{}
	if len(validation.IsDNS1123Subdomain(id.ID)) > 0 {
		err = apierrors.NewBadRequest("the subject is not a user id")
	} else {
		err = st.client.Get(ctx, client.ObjectKey{Name: id.ID}, user)
	}
	if err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) {
			// Builtin mode creates no user: a subject that is not a user of this apiserver has no keys here.
			klog.Warningf("delegated credential refused: user %s does not exist, actor %s", id.ID, id.Actor)
			return refuse(http.StatusUnauthorized, "invalid", ErrInvalidToken)
		}
		klog.ErrorS(err, "failed to read the user of a delegated credential", "user", id.ID)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code": http.StatusInternalServerError, "message": "failed to verify the credentials",
		})
		return nil, true, false
	}
	if (user.IsRestricted() || id.Restricted) && (req.IncludePlatformKey || req.IncludeVirtualKey) {
		klog.Warningf("keys refused for a restricted user: user %s, actor %q", id.ID, id.Actor)
		return refuse(http.StatusForbidden, "restricted", "the user is restricted")
	}
	klog.Infof("delegated credential accepted for verification: actor %s, user %s, jti %s", id.Actor, id.ID, id.Jti)
	// Everything said about the user is what the user's own record says, never what the token claims: the email is the
	// one the gateway virtual key is found by, so a token cannot point the lookup at another user's key.
	info := &UserInfo{
		Id:                user.Name,
		Name:              v1.GetUserName(user),
		Email:             v1.GetUserEmail(user),
		PreferredUsername: v1.GetAnnotation(user, v1.UserPreferredNameAnnotation),
	}
	if info.Name == "" {
		info.Name = user.Name
	}
	if !id.Expiry.IsZero() {
		info.Exp = id.Expiry.Unix()
	}
	return &externalVerdict{info: info, roles: user.Spec.Roles, userType: UserTypeDelegated, actor: id.Actor, jti: id.Jti}, true, true
}
