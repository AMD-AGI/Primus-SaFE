/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	apimetrics "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	// UserTypeExternal is the user type recorded for requests authenticated by the external chain.
	UserTypeExternal = "external"
	// UserTypeDelegated is the user type recorded for requests made with a delegated credential.
	UserTypeDelegated = "delegated"
)

// externalAuth is the state of the external mode. It is nil in builtin mode.
type externalAuth struct {
	chain    *extauth.Chain
	mappings []commonconfig.AuthGroupMapping
	client   client.Client
}

var externalState atomic.Pointer[externalAuth]

// InitExternalAuth builds the authenticator chain when auth.mode is external; in
// builtin mode (the default) it does nothing and ParseToken is unchanged. An
// external mode that cannot be built is an error: it must not start as builtin.
func InitExternalAuth(cli client.Client) error {
	if commonconfig.GetAuthMode() != commonconfig.AuthModeExternal {
		externalState.Store(nil)
		return nil
	}
	cfgs, err := commonconfig.GetAuthenticators()
	if err != nil {
		return err
	}
	mappings, err := commonconfig.GetAuthGroupMappings()
	if err != nil {
		return err
	}
	chain, err := buildChain(cfgs)
	if err != nil {
		return err
	}
	externalState.Store(&externalAuth{chain: chain, mappings: mappings, client: cli})
	klog.Infof("external authentication enabled with %d authenticator(s)", chain.Len())
	return nil
}

func buildChain(cfgs []commonconfig.AuthenticatorConfig) (*extauth.Chain, error) {
	if len(cfgs) == 0 {
		return nil, errors.New("auth.mode is external but auth.authenticators is empty")
	}
	var list []extauth.Authenticator
	for i, c := range cfgs {
		dur := func(name, v string) (time.Duration, error) {
			if v == "" {
				return 0, nil
			}
			d, err := time.ParseDuration(v)
			if err != nil {
				return 0, fmt.Errorf("auth.authenticators[%d].%s: %w", i, name, err)
			}
			return d, nil
		}
		switch strings.ToLower(c.Type) {
		case "oidc":
			skew, err := dur("clock_skew", c.ClockSkew)
			if err != nil {
				return nil, err
			}
			ttl, err := dur("jwks_cache_ttl", c.JWKSCacheTTL)
			if err != nil {
				return nil, err
			}
			a, err := extauth.NewOIDCAuthenticator(extauth.OIDCConfig{
				Name: c.Name, Issuer: c.Issuer, Audience: c.Audience, JWKSURL: c.JWKSURL,
				JWKSCacheTTL: ttl, ClockSkew: skew,
				IDClaim: c.IDClaim, UsernameClaim: c.UsernameClaim, EmailClaim: c.EmailClaim, GroupsClaim: c.GroupsClaim,
			})
			if err != nil {
				return nil, fmt.Errorf("auth.authenticators[%d]: %w", i, err)
			}
			list = append(list, a)
		case "webhook":
			var d [4]time.Duration
			for j, f := range []struct{ n, v string }{
				{"timeout", c.Timeout}, {"cache_ttl", c.CacheTTL}, {"cache_negative_ttl", c.CacheNegativeTTL}, {"stale_if_error", c.StaleIfError},
			} {
				var err error
				if d[j], err = dur(f.n, f.v); err != nil {
					return nil, err
				}
			}
			a, err := extauth.NewWebhookAuthenticator(extauth.WebhookConfig{
				Name: c.Name, URL: c.URL, CABundle: c.CABundle,
				Timeout: d[0], CacheTTL: d[1], CacheNegativeTTL: d[2], StaleIfError: d[3],
			})
			if err != nil {
				return nil, fmt.Errorf("auth.authenticators[%d]: %w", i, err)
			}
			list = append(list, a)
		case "builtin":
			list = append(list, extauth.Builtin{})
		default:
			return nil, fmt.Errorf("auth.authenticators[%d]: unknown type %q", i, c.Type)
		}
	}
	return extauth.NewChain(list...), nil
}

// parseExternal authenticates a request with the configured chain. handled is false
// when the chain reached the built-in mode and the caller must run it.
func parseExternal(c *gin.Context, st *externalAuth) (handled bool, err error) {
	cred := extauth.Credentials{ClientIP: c.ClientIP()}
	cred.Bearer = getBearerToken(c)
	if cookie, cerr := c.Cookie(CookieToken); cerr == nil {
		cred.Cookie = cookie
		cred.UserType, _ = c.Cookie(common.UserType)
	} else {
		cred.UserType = c.GetHeader("UserType")
	}
	if cred.Empty() {
		// Nothing to authenticate. The built-in mode, if it is in the chain, answers
		// "token not present" itself, as it always has.
		apimetrics.AuthFailuresTotal.WithLabelValues("missing").Inc()
		return true, commonerrors.NewUnauthorized("token not present")
	}

	id, aerr := st.chain.Authenticate(c.Request.Context(), cred)
	switch {
	case aerr == nil:
	case errors.Is(aerr, extauth.ErrBuiltin):
		return false, nil
	default:
		var unavailable *extauth.UnavailableError
		if errors.As(aerr, &unavailable) {
			klog.ErrorS(aerr, "authentication unavailable", "path", c.Request.URL.Path)
			apimetrics.AuthFailuresTotal.WithLabelValues("unavailable").Inc()
			// Never a pass: the caller is told to retry, not that its credential is bad.
			return true, apierrors.NewServiceUnavailable("the authentication service is unavailable")
		}
		apimetrics.AuthFailuresTotal.WithLabelValues("invalid").Inc()
		return true, commonerrors.NewUnauthorized(ErrInvalidToken)
	}

	if id.Delegated() {
		// Enforced here, for every caller of ParseToken, whoever issued the credential.
		if capability, forbidden := extauth.Forbidden(c.Request.Method, c.Request.URL.Path); forbidden {
			klog.Warningf("delegated credential refused: capability %s, actor %s, user %s", capability, id.Actor, id.ID)
			apimetrics.AuthFailuresTotal.WithLabelValues("delegated_forbidden").Inc()
			return true, commonerrors.NewForbidden("this operation is not available with a delegated token")
		}
		if !extauth.ScopeAllows(id.Scopes, c.Request.Method) {
			apimetrics.AuthFailuresTotal.WithLabelValues("delegated_scope").Inc()
			return true, commonerrors.NewForbidden("the token scope does not allow this operation")
		}
	}

	if err = st.ensureUser(c.Request.Context(), id); err != nil {
		klog.ErrorS(err, "failed to provision user", "user", id.ID)
		apimetrics.AuthFailuresTotal.WithLabelValues("provision").Inc()
		if apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) || errors.Is(err, errInvalidUserId) {
			return true, commonerrors.NewUnauthorized(ErrInvalidToken)
		}
		return true, err
	}

	overlay := extauth.MapGroups(id.ID, id.Groups, st.mappings, id.Restricted)
	c.Request = c.Request.WithContext(extauth.WithOverlay(c.Request.Context(), overlay))
	userType := UserTypeExternal
	if id.Delegated() {
		userType = UserTypeDelegated
		c.Set(common.ActorClientId, id.Actor)
		c.Set(common.TokenJti, id.Jti)
	}
	c.Set(common.UserId, id.ID)
	c.Set(common.UserName, id.Name)
	c.Set(common.UserType, userType)
	klog.Infof("external authentication accepted user %s (type %s)", id.ID, userType)
	return true, nil
}

var errInvalidUserId = errors.New("the authenticated user id is not a valid resource name")

// ensureUser creates the user record the first time an identity is seen, and keeps
// its name and email annotations current. Roles are not touched: the admission
// webhook gives a new user the default role, and group roles are computed per request.
func (st *externalAuth) ensureUser(ctx context.Context, id *extauth.Identity) error {
	if errs := validation.IsDNS1123Subdomain(id.ID); len(errs) > 0 {
		return errInvalidUserId
	}
	user := &v1.User{}
	err := st.client.Get(ctx, client.ObjectKey{Name: id.ID}, user)
	if err == nil {
		patch := client.MergeFrom(user.DeepCopy())
		changed := false
		for k, val := range map[string]string{v1.UserNameAnnotation: id.Name, v1.UserEmailAnnotation: id.Email} {
			if val != "" && v1.GetAnnotation(user, k) != val {
				v1.SetAnnotation(user, k, val)
				changed = true
			}
		}
		if changed {
			return st.client.Patch(ctx, user, patch)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	user = &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: id.ID,
			Annotations: map[string]string{
				v1.UserNameAnnotation:  id.Name,
				v1.UserEmailAnnotation: id.Email,
			},
		},
		Spec: v1.UserSpec{Type: v1.SSOUserType},
	}
	return client.IgnoreAlreadyExists(st.client.Create(ctx, user))
}

// IsDelegatedRequest reports whether the request was authenticated with a delegated credential.
func IsDelegatedRequest(c *gin.Context) bool {
	return c.GetString(common.UserType) == UserTypeDelegated && c.GetString(common.ActorClientId) != ""
}
