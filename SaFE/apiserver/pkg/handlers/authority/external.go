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
	warnStoredPlatformRoles(cfgs)
	return nil
}

// warnStoredPlatformRoles tells the operator at start-up what external mode does to the accounts
// the apiserver holds itself, so that a deployment that relied on a local administrator finds out
// when it starts and not during an outage of the identity provider: the platform roles stored on
// user objects stop counting, so such an account is an ordinary user. A builtin entry in the chain
// does not bring them back; it only keeps the credentials working.
func warnStoredPlatformRoles(cfgs []commonconfig.AuthenticatorConfig) {
	roles := make([]string, 0, len(extauth.PlatformRoles))
	for _, r := range extauth.PlatformRoles {
		roles = append(roles, string(r))
	}
	msg := fmt.Sprintf("auth.mode is external: the platform roles (%s) come only from auth.group_mappings; "+
		"the ones stored on user objects no longer count, so a local administrator is an ordinary user",
		strings.Join(roles, ", "))
	for _, c := range cfgs {
		if strings.EqualFold(c.Type, "builtin") {
			msg += "; the builtin authenticator in auth.authenticators still accepts the sessions and API keys of local " +
				"accounts, but as ordinary users, so no local account can administer the platform while the identity " +
				"provider is down (recovering means setting auth.mode back to builtin)"
			break
		}
	}
	klog.Warning(msg)
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
		case "introspection":
			var d [4]time.Duration
			for j, f := range []struct{ n, v string }{
				{"timeout", c.Timeout}, {"cache_ttl", c.CacheTTL}, {"cache_negative_ttl", c.CacheNegativeTTL}, {"stale_if_error", c.StaleIfError},
			} {
				var err error
				if d[j], err = dur(f.n, f.v); err != nil {
					return nil, err
				}
			}
			a, err := extauth.NewIntrospectionAuthenticator(extauth.IntrospectionConfig{
				Name: c.Name, URL: c.URL, Issuer: c.Issuer, CABundle: c.CABundle,
				Audience: c.Audience, AllowMissingAudience: c.AllowMissingAudience,
				ClientID: c.ClientID, ClientSecretFile: c.ClientSecretFile,
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

	id, builtin, aerr := st.authenticate(c.Request.Context(), cred, c.Request.URL.Path)
	if aerr != nil {
		return true, aerr
	}
	if builtin {
		return false, nil
	}

	if id.Delegated() {
		// Enforced here, for every caller of ParseToken, whoever issued the credential.
		if capability, forbidden := extauth.Forbidden(c.Request.Method, c.Request.URL.Path); forbidden {
			klog.Warningf("delegated credential refused: capability %s, actor %s, user %s", capability, id.Actor, id.ID)
			apimetrics.AuthFailuresTotal.WithLabelValues("delegated_forbidden").Inc()
			return true, commonerrors.NewForbidden("this operation is not available with a delegated token")
		}
		if !extauth.ScopeAllows(id.Scopes, c.Request) {
			apimetrics.AuthFailuresTotal.WithLabelValues("delegated_scope").Inc()
			return true, commonerrors.NewForbidden("the token scope does not allow this operation")
		}
	}

	if _, err = st.provision(c.Request.Context(), id); err != nil {
		return true, err
	}

	c.Request = c.Request.WithContext(extauth.WithOverlay(c.Request.Context(), st.overlay(id)))
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

// authenticate runs the chain on the credentials. builtin is true when the chain reached the
// built-in mode: no identity provider decided, and the caller runs the built-in path. A failure is
// returned as the error the caller answers with (401 for a credential that is rejected, 503 for an
// authenticator that could not decide), after it is counted.
func (st *externalAuth) authenticate(ctx context.Context, cred extauth.Credentials, path string) (id *extauth.Identity, builtin bool, err error) {
	id, aerr := st.chain.Authenticate(ctx, cred)
	switch {
	case aerr == nil:
		return id, false, nil
	case errors.Is(aerr, extauth.ErrBuiltin):
		return nil, true, nil
	}
	var unavailable *extauth.UnavailableError
	if errors.As(aerr, &unavailable) {
		klog.ErrorS(aerr, "authentication unavailable", "path", path)
		apimetrics.AuthFailuresTotal.WithLabelValues("unavailable").Inc()
		// Never a pass: the caller is told to retry, not that its credential is bad.
		return nil, false, apierrors.NewServiceUnavailable("the authentication service is unavailable")
	}
	apimetrics.AuthFailuresTotal.WithLabelValues("invalid").Inc()
	return nil, false, commonerrors.NewUnauthorized(ErrInvalidToken)
}

// provision makes sure the user record of an authenticated identity exists (ensureUser) and returns
// it. A failure is counted and returned as the error the caller answers with.
func (st *externalAuth) provision(ctx context.Context, id *extauth.Identity) (*v1.User, error) {
	user, err := st.ensureUser(ctx, id)
	if err != nil {
		klog.ErrorS(err, "failed to provision user", "user", id.ID)
		apimetrics.AuthFailuresTotal.WithLabelValues("provision").Inc()
		if apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) || errors.Is(err, errInvalidUserId) {
			return nil, commonerrors.NewUnauthorized(ErrInvalidToken)
		}
		return nil, err
	}
	return user, nil
}

// overlay is what the identity adds to its user record for the request: the roles and workspaces
// its groups map to, and its restriction.
func (st *externalAuth) overlay(id *extauth.Identity) *extauth.Overlay {
	return extauth.MapGroups(id.ID, id.Groups, st.mappings, id.Restricted)
}

var errInvalidUserId = errors.New("the authenticated user id is not a valid resource name")

// ensureUser creates the user record the first time an identity is seen. For a record
// that already exists it refreshes the name and email annotations, best effort: roles
// are not touched (the admission webhook gives a new user the default role, and group
// roles are computed per request), and a refresh that fails never fails the request.
// It returns the record as it is after that.
func (st *externalAuth) ensureUser(ctx context.Context, id *extauth.Identity) (*v1.User, error) {
	if errs := validation.IsDNS1123Subdomain(id.ID); len(errs) > 0 {
		return nil, errInvalidUserId
	}
	user := &v1.User{}
	err := st.client.Get(ctx, client.ObjectKey{Name: id.ID}, user)
	if err == nil {
		st.refreshUser(ctx, user, id)
		return user, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
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
	if err = client.IgnoreAlreadyExists(st.client.Create(ctx, user)); err != nil {
		return nil, err
	}
	return user, nil
}

// refreshUser makes the name and email annotations of an existing record follow the
// identity's claims. It is for the display only, so it is best effort: an error is
// logged and the request goes on. The account itself is never changed, and a record
// that is not an SSO user is left alone: the identity provider names an existing local
// account (Spec.Type default) by its user id, but the account stays what it is. Its
// user name is immutable, the user webhook refuses a patch that changes it, and a login
// through the provider is no reason to rewrite a record that SaFE manages.
func (st *externalAuth) refreshUser(ctx context.Context, user *v1.User, id *extauth.Identity) {
	if user.Spec.Type != v1.SSOUserType {
		return
	}
	patch := client.MergeFrom(user.DeepCopy())
	changed := false
	for k, val := range map[string]string{v1.UserNameAnnotation: id.Name, v1.UserEmailAnnotation: id.Email} {
		if val != "" && v1.GetAnnotation(user, k) != val {
			v1.SetAnnotation(user, k, val)
			changed = true
		}
	}
	if !changed {
		return
	}
	if err := st.client.Patch(ctx, user, patch); err != nil {
		klog.ErrorS(err, "failed to refresh the annotations of an existing user; continuing", "user", id.ID)
	}
}

// IsDelegatedRequest reports whether the request was authenticated with a delegated credential.
func IsDelegatedRequest(c *gin.Context) bool {
	return c.GetString(common.UserType) == UserTypeDelegated && c.GetString(common.ActorClientId) != ""
}
