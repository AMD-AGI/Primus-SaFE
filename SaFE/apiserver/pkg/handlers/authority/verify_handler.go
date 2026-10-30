/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	apimetrics "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/apikey"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// VerifyTokenRequest represents the request body for token verification
type VerifyTokenRequest struct {
	// Raw cookie header string from user request (e.g., "Token=xxx; UserType=sso")
	Cookie string `json:"cookie,omitempty"`
	// Alternative: Authorization header value (e.g., "Bearer xxx" or "Bearer ak-xxx")
	Authorization string `json:"authorization,omitempty"`
	// User type when using authorization header
	UserType string `json:"userType,omitempty"`
	// API key for direct authentication (e.g., "ak-xxx")
	ApiKey string `json:"apiKey,omitempty"`
	// Original client IP, used for API key whitelist validation
	ClientIP string `json:"clientIP,omitempty"`
	// If true, returns the user's platform API key (GetOrCreate).
	// NOTE: this platform key is effectively a "shadow" of the user's API key:
	// it is a stable per-user ak-... minted/fetched via GetOrCreate and is NOT
	// the specific ak-... presented for this verification.
	IncludePlatformKey bool `json:"includePlatformKey,omitempty"`
	// If true, returns the user's LiteLLM virtual key (decrypted)
	IncludeVirtualKey bool `json:"includeVirtualKey,omitempty"`
}

// VerifyTokenResponse represents the response body for token verification
type VerifyTokenResponse struct {
	Id            string        `json:"id"`
	Name          string        `json:"name"`
	Email         string        `json:"email,omitempty"`
	PreferredName string        `json:"preferredName,omitempty"`
	Exp           int64         `json:"exp"`
	Type          string        `json:"type"`
	Roles         []v1.UserRole `json:"roles"`
	ApiKeyId      int64         `json:"apiKeyId,omitempty"`
	PlatformKey   string        `json:"platformKey,omitempty"`
	VirtualKey    string        `json:"virtualKey,omitempty"`
}

// platformKeyFetcher resolves the platform API key for a user. It is a package
// variable so tests can substitute a stub without patching the DB layer.
var platformKeyFetcher = fetchPlatformKey

// fetchPlatformKey fetches or creates the platform API key from the database.
// Returns an empty key (no error) when the database is unavailable.
func fetchPlatformKey(ctx context.Context, userId, userName string) (string, error) {
	if !commonconfig.IsDBEnable() {
		klog.Warning("database not enabled, cannot provide platform key")
		return "", nil
	}
	db := dbclient.NewClient()
	if db == nil {
		klog.Warning("database client not available, cannot provide platform key")
		return "", nil
	}
	return apikey.GetOrCreatePlatformKey(ctx, db, userId, userName)
}

// VerifyToken validates a user token and returns user information
// This endpoint is only accessible to internal services with valid X-Internal-Token
func VerifyToken(c *gin.Context) {
	// Validate internal service authentication
	internalAuth := InternalAuthInstance()
	if internalAuth == nil {
		klog.Error("internal auth not initialized")
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "internal auth not available",
		})
		return
	}

	internalToken := c.GetHeader(InternalAuthTokenHeader)
	if !internalAuth.Validate(internalToken) {
		klog.Warning("invalid internal token for verify request")
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": "internal authentication required",
		})
		return
	}

	// Parse request body
	var req VerifyTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		klog.ErrorS(err, "failed to parse verify token request")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "invalid request body",
		})
		return
	}

	// Authenticate: in external mode the configured chain decides first, as it does for a request;
	// when the chain reaches the built-in mode, or the mode is builtin (the default), the apiserver's
	// own credentials are checked: Cookie > ApiKey > Authorization (auto-detect ak- prefix).
	var userInfo *UserInfo
	var userType string
	var roles []v1.UserRole
	external := false
	if st := externalState.Load(); st != nil {
		verdict, builtin, ok := verifyExternal(c, st, &req)
		if !ok {
			return
		}
		if !builtin {
			userInfo, userType, roles, external = verdict.info, verdict.userType, verdict.roles, true
		}
	}
	if !external {
		var ok bool
		if userInfo, userType, ok = verifyBuiltin(c, &req); !ok {
			return
		}
		// The roles as the apiserver sees them: in external mode the stored platform roles do not count.
		roles = builtinRoles(userInfo.Id, userInfo.Roles)
	}

	resp := VerifyTokenResponse{
		Id:            userInfo.Id,
		Name:          userInfo.Name,
		Email:         userInfo.Email,
		PreferredName: userInfo.PreferredUsername,
		Exp:           userInfo.Exp,
		Type:          userType,
		ApiKeyId:      userInfo.ApiKeyId,
		Roles:         roles,
	}

	// Backfill preferred name from the User CR when the auth path (e.g. API key)
	// didn't carry it. Mirrors the email backfill below.
	if resp.PreferredName == "" {
		if tokenInst := DefaultTokenInstance(); tokenInst != nil {
			if user, err := getUserById(c.Request.Context(), tokenInst, userInfo.Id); err == nil {
				resp.PreferredName = v1.GetAnnotation(user, v1.UserPreferredNameAnnotation)
			}
		}
	}

	// Optionally include platform API key (GetOrCreate)
	if req.IncludePlatformKey {
		if platformKey, err := platformKeyFetcher(c.Request.Context(), userInfo.Id, userInfo.Name); err != nil {
			klog.ErrorS(err, "failed to get/create platform key", "userId", userInfo.Id)
		} else if platformKey != "" {
			resp.PlatformKey = platformKey
		}
	}

	// Optionally include LiteLLM virtual key (decrypted from DB)
	if req.IncludeVirtualKey {
		// If email is missing (e.g., API key auth), try to fetch from User CR
		if resp.Email == "" {
			if tokenInst := DefaultTokenInstance(); tokenInst != nil {
				if user, err := getUserById(c.Request.Context(), tokenInst, userInfo.Id); err == nil {
					resp.Email = v1.GetUserEmail(user)
				}
			}
		}
		if resp.Email != "" {
			apiKeyToken := ApiKeyTokenInstance()
			if apiKeyToken != nil {
				virtualKey, err := apiKeyToken.GetVirtualKeyByEmail(c.Request.Context(), resp.Email)
				if err != nil {
					klog.ErrorS(err, "failed to get virtual key", "email", resp.Email)
				} else if virtualKey != "" {
					resp.VirtualKey = virtualKey
				}
			} else {
				klog.Warning("API key auth not initialized, cannot provide virtual key")
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": resp,
	})
}

// verifyBuiltin authenticates the credentials of a verify request with the apiserver's own sessions,
// tokens and API keys. It writes the failure answer to the gin context and returns ok false when
// the credentials are not accepted.
func verifyBuiltin(c *gin.Context, req *VerifyTokenRequest) (userInfo *UserInfo, userType string, ok bool) {
	// Authenticate: Cookie > ApiKey > Authorization (auto-detect ak- prefix)
	var rawToken string
	var err error

	if req.Cookie != "" {
		rawToken, userType, err = parseCookieString(req.Cookie)
		if err != nil {
			klog.ErrorS(err, "failed to parse cookie string")
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":    http.StatusBadRequest,
				"message": "invalid cookie format",
			})
			return nil, "", false
		}
	} else if req.ApiKey != "" {
		userInfo, err = verifyApiKey(c, req.ApiKey, req.ClientIP)
		if err != nil {
			return nil, "", false
		}
		userType = UserTypeApiKey
	} else if req.Authorization != "" {
		if apiKey := ExtractApiKeyFromRequest(req.Authorization); apiKey != "" {
			userInfo, err = verifyApiKey(c, apiKey, req.ClientIP)
			if err != nil {
				return nil, "", false
			}
			userType = UserTypeApiKey
		} else {
			rawToken = extractBearerToken(req.Authorization)
			userType = req.UserType
			if userType == "" {
				userType = string(v1.DefaultUserType)
			}
		}
	} else {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "cookie, apiKey, or authorization is required",
		})
		return nil, "", false
	}

	// For cookie/bearer-token paths, validate the token to get userInfo
	if userInfo == nil {
		if rawToken == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": ErrInvalidToken,
			})
			return nil, "", false
		}

		var tokenInstance TokenInterface
		if userType == string(v1.SSOUserType) {
			if !commonconfig.IsSSOEnable() {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"code":    http.StatusBadRequest,
					"message": "SSO is not enabled",
				})
				return nil, "", false
			}
			tokenInstance = SSOInstance()
		} else {
			tokenInstance = DefaultTokenInstance()
		}

		if tokenInstance == nil {
			klog.Error("token validator not available")
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":    http.StatusInternalServerError,
				"message": "token validator not available",
			})
			return nil, "", false
		}

		userInfo, err = tokenInstance.Validate(c.Request.Context(), rawToken)
		if err != nil {
			klog.ErrorS(err, "failed to validate user token")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": ErrInvalidToken,
			})
			return nil, "", false
		}
	}

	return userInfo, userType, true
}

// externalVerdict is what the external chain decided about the credentials of a verify request.
type externalVerdict struct {
	info *UserInfo
	// roles are the roles of the user as a request authenticated with the same credentials sees
	// them (extauth.ApplyOverlay): the platform roles come from the identity's groups only.
	roles []v1.UserRole
	// userType is UserTypeExternal, or UserTypeDelegated for a delegated token of a key actor.
	userType string
}

// verifyExternal authenticates the credentials of a verify request with the configured chain, the
// way ParseToken does for a request, so an identity provider's credential gets the roles its
// groups map to and the services that authorize through POST /auth/verify can see an
// administrator there. builtin is true when the chain reached the built-in mode: the caller then
// checks the apiserver's own credentials. It writes the failure answer to the gin context and
// returns ok false when the credentials are not accepted.
func verifyExternal(c *gin.Context, st *externalAuth, req *VerifyTokenRequest) (verdict *externalVerdict, builtin, ok bool) {
	// The same priority as the built-in path: Cookie > ApiKey > Authorization.
	cred := extauth.Credentials{ClientIP: req.ClientIP}
	if cred.ClientIP == "" {
		cred.ClientIP = c.ClientIP()
	}
	switch {
	case req.Cookie != "":
		var err error
		if cred.Cookie, cred.UserType, err = parseCookieString(req.Cookie); err != nil {
			klog.ErrorS(err, "failed to parse cookie string")
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":    http.StatusBadRequest,
				"message": "invalid cookie format",
			})
			return nil, false, false
		}
	case req.ApiKey != "":
		cred.Bearer = req.ApiKey
	case req.Authorization != "":
		if cred.Bearer = ExtractApiKeyFromRequest(req.Authorization); cred.Bearer == "" {
			cred.Bearer, cred.UserType = extractBearerToken(req.Authorization), req.UserType
		}
	default:
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "cookie, apiKey, or authorization is required",
		})
		return nil, false, false
	}
	if cred.Empty() {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": ErrInvalidToken,
		})
		return nil, false, false
	}

	ctx := c.Request.Context()
	id, builtin, err := st.authenticate(ctx, cred, c.Request.URL.Path)
	if err != nil {
		abortVerify(c, err)
		return nil, false, false
	}
	if builtin {
		return nil, true, true
	}
	userType := UserTypeExternal
	if id.Delegated() {
		// Token verification is one of the capabilities a delegated credential does not have
		// (extauth.Forbidden): the verdict hands the caller the user's keys. The exception is a party the
		// operator named in auth.key_actors, holding a token with the safe.api scope: the verdict is about
		// the token's subject, whom the chain authenticated, and about nobody else; the request has no
		// field that names another user. The token's audience is the one the authenticator that accepted it
		// requires.
		if !st.isKeyActor(id) {
			klog.Warningf("delegated credential refused: capability %s, actor %s, user %s", extauth.CapabilityPlatformKey, id.Actor, id.ID)
			apimetrics.AuthFailuresTotal.WithLabelValues("delegated_forbidden").Inc()
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": "this operation is not available with a delegated token",
			})
			return nil, false, false
		}
		if !extauth.HasScope(id.Scopes, extauth.ScopeAPI) {
			klog.Warningf("delegated credential refused: scope %v does not include %s, actor %s, user %s", id.Scopes, extauth.ScopeAPI, id.Actor, id.ID)
			apimetrics.AuthFailuresTotal.WithLabelValues("delegated_scope").Inc()
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": "the token scope does not allow this operation",
			})
			return nil, false, false
		}
		klog.Infof("delegated credential accepted for verification: actor %s, user %s, jti %s", id.Actor, id.ID, id.Jti)
		userType = UserTypeDelegated
	}
	user, err := st.provision(ctx, id)
	if err != nil {
		abortVerify(c, err)
		return nil, false, false
	}

	info := &UserInfo{
		Id:                id.ID,
		Name:              id.Name,
		Email:             id.Email,
		PreferredUsername: v1.GetAnnotation(user, v1.UserPreferredNameAnnotation),
	}
	if info.Email == "" {
		info.Email = v1.GetUserEmail(user)
	}
	if id.Delegated() {
		// The keys of a delegated verdict are the subject's: the gateway virtual key is found by the email,
		// and for the subject that is the one on its own record. For a local account that is the email it was
		// created with; for an account the identity provider created the record follows the provider's claim
		// (ensureUser), so the provider is trusted for the pairing of a subject and its email, as it is for the
		// subject itself and for every credential this endpoint verifies.
		if stored := v1.GetUserEmail(user); stored != "" {
			info.Email = stored
		}
	}
	if !id.Expiry.IsZero() {
		info.Exp = id.Expiry.Unix()
	}
	roles := extauth.ApplyOverlay(extauth.WithOverlay(ctx, st.overlay(id)), user).Spec.Roles
	return &externalVerdict{info: info, roles: roles, userType: userType}, false, true
}

// abortVerify answers a verify request with the failure the shared external path returned: 401 for a
// rejected credential, 503 when the authentication service could not decide, and 500 for anything else.
func abortVerify(c *gin.Context, err error) {
	status, message := http.StatusInternalServerError, "failed to verify the credentials"
	var statusErr apierrors.APIStatus
	if errors.As(err, &statusErr) {
		switch statusErr.Status().Code {
		case http.StatusUnauthorized:
			status, message = http.StatusUnauthorized, ErrInvalidToken
		case http.StatusServiceUnavailable:
			status, message = http.StatusServiceUnavailable, "the authentication service is unavailable"
		}
	}
	c.AbortWithStatusJSON(status, gin.H{"code": status, "message": message})
}

// verifyApiKey validates an API key and returns user info.
// Sends error response to gin context on failure.
func verifyApiKey(c *gin.Context, apiKey, clientIP string) (*UserInfo, error) {
	apiKeyToken := ApiKeyTokenInstance()
	if apiKeyToken == nil {
		klog.Error("API key auth not initialized")
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "API key authentication not available",
		})
		return nil, commonerrors.NewInternalError("API key auth not initialized")
	}

	userInfo, err := apiKeyToken.ValidateApiKey(c.Request.Context(), apiKey, clientIP)
	if err != nil {
		klog.ErrorS(err, "failed to validate API key")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "invalid API key",
		})
		return nil, err
	}
	return userInfo, nil
}

// parseCookieString parses raw cookie string and extracts token and userType
func parseCookieString(cookieStr string) (token, userType string, err error) {
	cookies := strings.Split(cookieStr, ";")
	for _, cookie := range cookies {
		cookie = strings.TrimSpace(cookie)
		if cookie == "" {
			continue
		}

		parts := strings.SplitN(cookie, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case CookieToken:
			token = value
		case CookieUserType:
			userType = value
		}
	}

	if token == "" {
		return "", "", commonerrors.NewBadRequest("token not found in cookie")
	}

	return token, userType, nil
}

// extractBearerToken extracts the token from an Authorization header value
func extractBearerToken(authHeader string) string {
	if authHeader == "" {
		return ""
	}
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return parts[1]
}
