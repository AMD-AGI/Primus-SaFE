/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
)

var (
	ssoInitOnce sync.Once
	ssoInstance *ssoToken

	DefaultOIDCScopes = []string{oidc.ScopeOpenID, "profile", "email"}
)

// ssoToken implements TokenInterface for OAuth2/OpenID Connect authentication
type ssoToken struct {
	endpoint     string
	clientId     string
	clientSecret string
	redirectURI  string

	client.Client
	httpClient httpclient.Interface
	dbClient   dbclient.Interface
	provider   *oidc.Provider
	verifier   *oidc.IDTokenVerifier

	// login is the provider of sso.login, where the console signs in when it is enabled; nil otherwise.
	// The fields above are then the provider of sso.secret_path, which may be absent: it keeps serving
	// the logins and sessions of other web apps that bring its codes until it is removed. With sso.login
	// on, it is discovered on first use (secretLazy), so that it being down does not stop the console's
	// sign-in; with sso.login off it is discovered at start-up, as before (provider, verifier).
	login      *loginProvider
	secretLazy *loginProvider
}

// discoveryRetryInterval is how long a provider that could not be discovered is not asked again: the
// requests that need it meanwhile fail at once instead of each waiting for the provider.
const discoveryRetryInterval = 10 * time.Second

// loginProvider is an OpenID Connect provider discovered on first use. It is the provider of sso.login,
// where the console signs in as a public client: there is no client secret, and the authorization code
// is bound to the browser that asked for it by PKCE (RFC 7636, S256). The ID token it returns is the
// session's token, as the token of the sso.secret_path provider is, so sessions and users are kept
// exactly as before. With sso.login on, the sso.secret_path provider is one too (no redirectURI here).
//
// Discovery happens at start-up and, if the provider cannot be reached then, on first use until it
// succeeds, at most once per retryInterval: a provider that is down while the apiserver starts does
// not stop the apiserver.
type loginProvider struct {
	issuer      string
	clientId    string
	redirectURI string
	httpClient  httpclient.Interface
	// retryInterval is discoveryRetryInterval; tests set it to 0.
	retryInterval time.Duration

	mu        sync.Mutex
	provider  *oidc.Provider
	verifier  *oidc.IDTokenVerifier
	failedAt  time.Time
	lastError error
}

// get returns the discovered provider and its ID token verifier, discovering them if needed.
func (p *loginProvider) get() (*oidc.Provider, *oidc.IDTokenVerifier, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider != nil {
		return p.provider, p.verifier, nil
	}
	if p.lastError != nil && time.Since(p.failedAt) < p.retryInterval {
		return nil, nil, p.lastError
	}
	// The context is kept by the provider for its later key set fetches, so it is not a request's.
	ctx := oidc.ClientContext(context.Background(), p.httpClient.GetBaseClient())
	start := time.Now()
	provider, err := oidc.NewProvider(ctx, p.issuer)
	metrics.ObserveIdPRequest(metrics.OpProviderDiscovery, start, err)
	if err != nil {
		p.failedAt, p.lastError = time.Now(), fmt.Errorf("failed to discover the SSO provider %q: %v", p.issuer, err)
		return nil, nil, p.lastError
	}
	p.provider = provider
	p.verifier = provider.Verifier(&oidc.Config{ClientID: p.clientId})
	return p.provider, p.verifier, nil
}

// secret returns the provider of sso.secret_path and its ID token verifier: discovered at start-up when
// sso.login is off (as before), on first use when it is on.
func (c *ssoToken) secret() (*oidc.Provider, *oidc.IDTokenVerifier, error) {
	if c.secretLazy != nil {
		return c.secretLazy.get()
	}
	if c.verifier == nil {
		return nil, nil, fmt.Errorf("no provider is configured for it")
	}
	return c.provider, c.verifier, nil
}

// NewSSOToken creates and returns a singleton instance of OAuth2 token handler for sso
// implementing the TokenInterface for SSO user authentication
func NewSSOToken(cli client.Client) *ssoToken {
	ssoInitOnce.Do(func() {
		var err error
		ssoInstance, err = initializeSSOToken(cli)
		if err != nil {
			klog.ErrorS(err, "failed to init sso token")
		}
	})
	return ssoInstance
}

// SSOInstance returns the singleton instance of ssoToken
func SSOInstance() *ssoToken {
	return ssoInstance
}

// initializeSSOToken initializes and returns a new ssoToken instance
func initializeSSOToken(cli client.Client) (*ssoToken, error) {
	var dbClient *dbclient.Client
	if commonconfig.IsDBEnable() {
		if dbClient = dbclient.NewClient(); dbClient == nil {
			return nil, fmt.Errorf("failed to new db client")
		}
	}
	ssoTokenInstance := &ssoToken{
		endpoint:     commonconfig.GetSSOEndpoint(),
		clientId:     commonconfig.GetSSOClientId(),
		clientSecret: commonconfig.GetSSOClientSecret(),
		redirectURI:  commonconfig.GetSSORedirectURI(),
		Client:       cli,
		httpClient:   httpclient.NewClient(),
		dbClient:     dbClient,
	}
	if commonconfig.IsSSOLoginEnable() {
		login, err := newLoginProvider(ssoTokenInstance.httpClient)
		if err != nil {
			return nil, err
		}
		ssoTokenInstance.login = login
		if _, _, err = login.get(); err != nil {
			klog.ErrorS(err, "sso.login: the provider cannot be discovered now; trying again on first use")
		}
		// The provider of sso.secret_path is optional now: without it, only the console's sign-in exists.
		if ssoTokenInstance.endpoint == "" && ssoTokenInstance.clientId == "" &&
			ssoTokenInstance.clientSecret == "" && ssoTokenInstance.redirectURI == "" {
			klog.Info("sso: the console signs in at the sso.login provider; no sso.secret_path provider is configured")
			return ssoTokenInstance, nil
		}
		if ssoTokenInstance.endpoint == "" || ssoTokenInstance.clientId == "" ||
			ssoTokenInstance.clientSecret == "" || ssoTokenInstance.redirectURI == "" {
			return nil, fmt.Errorf("failed to find sso config")
		}
		// Discovered on first use: the console's sign-in does not depend on this provider being up.
		ssoTokenInstance.secretLazy = &loginProvider{issuer: ssoTokenInstance.endpoint, clientId: ssoTokenInstance.clientId,
			httpClient: ssoTokenInstance.httpClient, retryInterval: discoveryRetryInterval}
		if _, _, err = ssoTokenInstance.secretLazy.get(); err != nil {
			klog.ErrorS(err, "sso: the sso.secret_path provider cannot be discovered now; trying again on first use")
		}
		return ssoTokenInstance, nil
	}
	// Validate required configuration
	if ssoTokenInstance.endpoint == "" || ssoTokenInstance.clientId == "" ||
		ssoTokenInstance.clientSecret == "" || ssoTokenInstance.redirectURI == "" {
		return nil, fmt.Errorf("failed to find sso config")
	}

	ctx := oidc.ClientContext(context.Background(), ssoTokenInstance.httpClient.GetBaseClient())
	var err error
	discoveryStart := time.Now()
	ssoTokenInstance.provider, err = oidc.NewProvider(ctx, ssoTokenInstance.endpoint)
	metrics.ObserveIdPRequest(metrics.OpProviderDiscovery, discoveryStart, err)
	if err != nil {
		return nil, fmt.Errorf("failed to new provider %q: %v", ssoTokenInstance.endpoint, err)
	}
	// Configure ID token verifier
	ssoTokenInstance.verifier = ssoTokenInstance.provider.Verifier(
		&oidc.Config{ClientID: ssoTokenInstance.clientId})
	return ssoTokenInstance, nil
}

// newLoginProvider reads sso.login. All three settings are required, and the redirect address must be
// absolute: it is compared exactly by the provider, at the authorization request and at the exchange.
func newLoginProvider(httpClient httpclient.Interface) (*loginProvider, error) {
	p := &loginProvider{
		issuer:        commonconfig.GetSSOLoginIssuer(),
		clientId:      commonconfig.GetSSOLoginClientId(),
		redirectURI:   commonconfig.GetSSOLoginRedirectURI(),
		httpClient:    httpClient,
		retryInterval: discoveryRetryInterval,
	}
	if p.issuer == "" || p.clientId == "" || p.redirectURI == "" {
		return nil, fmt.Errorf("sso.login.enable needs sso.login.issuer, sso.login.client_id and sso.login.redirect_uri")
	}
	if u, err := url.Parse(p.redirectURI); err != nil || !u.IsAbs() || u.Host == "" {
		return nil, fmt.Errorf("sso.login.redirect_uri %q must be an absolute URL", p.redirectURI)
	}
	return p, nil
}

// codeVerifierPattern is the form of a PKCE code verifier (RFC 7636 section 4.1).
var codeVerifierPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

// Login exchanges authorization code for access token and ID token
// Implements TokenInterface.Login method for OAuth2 flow.
//
// A code that comes with a PKCE code verifier was asked for by the console at the sso.login provider
// and is exchanged there; any other code is the sso.secret_path provider's, as before (the code other
// web apps receive through the SSO bridge).
func (c *ssoToken) Login(ctx context.Context, input TokenInput) (*v1.User, *TokenResponse, error) {
	if input.Code == "" {
		metrics.ObserveLoginFailure(metrics.StageBadRequest)
		return nil, nil, commonerrors.NewBadRequest("no code in request")
	}
	if c.httpClient == nil {
		metrics.ObserveLoginFailure(metrics.StageBadRequest)
		return nil, nil, commonerrors.NewInternalError("http client is nil")
	}
	var (
		rawIDToken string
		userInfo   *UserInfo
		err        error
	)
	if input.CodeVerifier != "" {
		rawIDToken, userInfo, err = c.exchangeAtLoginProvider(ctx, input)
	} else {
		rawIDToken, userInfo, err = c.exchangeAtSecretProvider(ctx, input.Code)
	}
	if err != nil {
		return nil, nil, err
	}

	// Synchronize user with system
	user, err := c.synchronizeUser(ctx, userInfo)
	if err != nil {
		metrics.ObserveLoginFailure(metrics.StageUserSync)
		return nil, nil, err
	}
	userToken := ""
	if commonconfig.IsDBEnable() {
		if userToken, err = c.updateUserInfoInDB(ctx, rawIDToken, userInfo); err != nil {
			metrics.ObserveLoginFailure(metrics.StageDBSession)
			return nil, nil, err
		}
	} else {
		userToken = rawIDToken
	}
	metrics.ObserveLoginSuccess()
	response := &TokenResponse{
		Expire: userInfo.Exp,
		Token:  userToken,
	}
	return user, response, nil
}

// exchangeAtSecretProvider redeems a code of the sso.secret_path provider with the client secret.
func (c *ssoToken) exchangeAtSecretProvider(ctx context.Context, code string) (string, *UserInfo, error) {
	if c.provider == nil && c.secretLazy == nil {
		metrics.ObserveLoginFailure(metrics.StageBadRequest)
		return "", nil, commonerrors.NewBadRequest("this sign-in is not configured: the code must come with a code verifier")
	}
	provider, _, err := c.secret()
	if err != nil {
		metrics.ObserveLoginFailure(metrics.StageTokenExchange)
		return "", nil, commonerrors.NewInternalError(err.Error())
	}
	ctx = oidc.ClientContext(ctx, c.httpClient.GetBaseClient())
	config := c.oauth2Config(provider)
	exchangeStart := time.Now()
	token, err := config.Exchange(ctx, code)
	metrics.ObserveIdPRequest(metrics.OpTokenExchange, exchangeStart, err)
	if err != nil {
		metrics.ObserveLoginFailure(metrics.StageTokenExchange)
		return "", nil, commonerrors.NewInternalError(fmt.Sprintf("failed to get token: %v", err))
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		metrics.ObserveLoginFailure(metrics.StageTokenExchange)
		return "", nil, commonerrors.NewInternalError("no id_token in token response")
	}

	// Validate ID token and extract user info
	userInfo, err := c.validate(ctx, rawIDToken)
	if err != nil {
		metrics.ObserveLoginFailure(metrics.StageTokenVerify)
		return "", nil, err
	}
	return rawIDToken, userInfo, nil
}

// exchangeAtLoginProvider redeems a code of the sso.login provider as a public client: the client id
// in the form, no secret, and the PKCE verifier the console kept since it sent the challenge.
func (c *ssoToken) exchangeAtLoginProvider(ctx context.Context, input TokenInput) (string, *UserInfo, error) {
	if c.login == nil {
		metrics.ObserveLoginFailure(metrics.StageBadRequest)
		return "", nil, commonerrors.NewBadRequest("a code verifier was sent, but sso.login is not enabled")
	}
	if !codeVerifierPattern.MatchString(input.CodeVerifier) {
		metrics.ObserveLoginFailure(metrics.StageBadRequest)
		return "", nil, commonerrors.NewBadRequest("invalid code verifier")
	}
	provider, verifier, err := c.login.get()
	if err != nil {
		metrics.ObserveLoginFailure(metrics.StageTokenExchange)
		return "", nil, commonerrors.NewInternalError(err.Error())
	}
	config := &oauth2.Config{
		ClientID: c.login.clientId,
		Endpoint: oauth2.Endpoint{
			AuthURL:   provider.Endpoint().AuthURL,
			TokenURL:  provider.Endpoint().TokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		Scopes:      DefaultOIDCScopes,
		RedirectURL: c.login.redirectURI,
	}
	ctx = oidc.ClientContext(ctx, c.httpClient.GetBaseClient())
	exchangeStart := time.Now()
	token, err := config.Exchange(ctx, input.Code, oauth2.VerifierOption(input.CodeVerifier))
	metrics.ObserveIdPRequest(metrics.OpTokenExchange, exchangeStart, err)
	if err != nil {
		metrics.ObserveLoginFailure(metrics.StageTokenExchange)
		return "", nil, commonerrors.NewInternalError(fmt.Sprintf("failed to get token: %v", err))
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		metrics.ObserveLoginFailure(metrics.StageTokenExchange)
		return "", nil, commonerrors.NewInternalError("no id_token in token response")
	}
	userInfo, err := c.verifyWith(ctx, verifier, rawIDToken)
	if err != nil {
		metrics.ObserveLoginFailure(metrics.StageTokenVerify)
		return "", nil, err
	}
	return rawIDToken, userInfo, nil
}

// Validate verifies the user token based on the provided rawToken
// If database is enabled, treats rawToken as session-id and retrieves the actual token from database first
// Then validates the retrieved token through OIDC provider to extract user information
// Returns user info from validated token or appropriate error
func (c *ssoToken) Validate(ctx context.Context, rawToken string) (*UserInfo, error) {
	if commonconfig.IsDBEnable() {
		dbTags := dbclient.GetUserTokenFieldTags()
		dbSql := sqrl.And{
			sqrl.Eq{dbclient.GetFieldTag(dbTags, "SessionId"): rawToken},
		}
		nowTime := time.Now().Unix()
		dbSql = append(dbSql, sqrl.Gt{dbclient.GetFieldTag(dbTags, "ExpireTime"): nowTime})
		userToken, err := c.dbClient.SelectUserTokens(ctx, dbSql, nil, 1, 0)
		if err != nil || len(userToken) == 0 {
			return nil, commonerrors.NewUnauthorized("token not present")
		}
		rawToken = userToken[0].Token
	}
	// A session made by a sign-in at the sso.login provider holds that provider's ID token.
	if c.login != nil && unverifiedIssuer(rawToken) == c.login.issuer {
		_, verifier, err := c.login.get()
		if err != nil {
			return nil, err
		}
		return c.verifyWith(ctx, verifier, rawToken)
	}
	return c.validate(ctx, rawToken)
}

// unverifiedIssuer reads the iss claim of a JWT without verifying it. It only chooses which provider's
// verifier judges the token; that verifier checks the issuer again, with the signature.
func unverifiedIssuer(raw string) string {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Issuer string `json:"iss"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	return claims.Issuer
}

// validate verifies ID token and extracts user information
// Implements TokenInterface.Validate method for OAuth2 tokens
func (c *ssoToken) validate(ctx context.Context, rawToken string) (*UserInfo, error) {
	_, verifier, err := c.secret()
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %v", err)
	}
	return c.verifyWith(ctx, verifier, rawToken)
}

// verifyWith verifies an ID token with a provider's verifier and extracts the user information.
func (c *ssoToken) verifyWith(ctx context.Context, verifier *oidc.IDTokenVerifier, rawToken string) (*UserInfo, error) {
	idToken, err := verifier.Verify(ctx, rawToken)
	metrics.ObserveTokenVerify(err)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %v", err)
	}

	var claims json.RawMessage
	if err = idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to decode ID token claims: %v", err)
	}

	buff := new(bytes.Buffer)
	if err = json.Indent(buff, claims, "", "  "); err != nil {
		return nil, fmt.Errorf("failed to indent ID token claims: %v", err)
	}
	// klog.Infof("user buffer: %s, tokensize: %d", buff.String(), len(rawToken))
	userInfo := &UserInfo{}
	err = json.Unmarshal(buff.Bytes(), userInfo)
	if err != nil {
		return nil, err
	}
	userInfo.Id = generateSSOUserId(userInfo.Sub, userInfo.Email)

	user := &v1.User{}
	if c.Get(ctx, client.ObjectKey{Name: userInfo.Id}, user) == nil {
		userInfo.Roles = user.Spec.Roles
	}
	return userInfo, nil
}

// synchronizeUser creates or updates user based on OAuth2 user information
func (c *ssoToken) synchronizeUser(ctx context.Context, userInfo *UserInfo) (*v1.User, error) {
	user, err := getUserById(ctx, c.Client, userInfo.Id)
	if err == nil {
		patch := client.MergeFrom(user.DeepCopy())
		isChanged := false
		if v1.GetUserName(user) != userInfo.Name {
			v1.SetAnnotation(user, v1.UserNameAnnotation, userInfo.Name)
			isChanged = true
		}
		if v1.GetUserEmail(user) != userInfo.Email {
			v1.SetAnnotation(user, v1.UserEmailAnnotation, userInfo.Email)
			isChanged = true
		}
		if v1.GetAnnotation(user, v1.UserPreferredNameAnnotation) != userInfo.PreferredUsername {
			v1.SetAnnotation(user, v1.UserPreferredNameAnnotation, userInfo.PreferredUsername)
			isChanged = true
		}
		if isChanged {
			if err = c.Patch(ctx, user, patch); err != nil {
				return nil, err
			}
		}
	} else {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		// Create new user
		user = &v1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name: userInfo.Id,
				Annotations: map[string]string{
					v1.UserNameAnnotation:          userInfo.Name,
					v1.UserEmailAnnotation:         userInfo.Email,
					v1.UserPreferredNameAnnotation: userInfo.PreferredUsername,
				},
			},
			Spec: v1.UserSpec{
				Type: v1.SSOUserType,
			},
		}
		if err = c.Create(ctx, user); err != nil {
			return nil, client.IgnoreAlreadyExists(err)
		}
	}
	return user, err
}

// updateUserInfoInDB updates user token information in database
// Generates a new session ID and stores user token with expiration time
// Returns the session ID for successful update
func (c *ssoToken) updateUserInfoInDB(ctx context.Context, rawIDToken string, userInfo *UserInfo) (string, error) {
	sessionId := string(uuid.NewUUID())
	err := c.dbClient.UpsertUserToken(ctx, &dbclient.UserToken{
		UserId:       userInfo.Id,
		SessionId:    sessionId,
		Token:        rawIDToken,
		CreationTime: time.Now().UTC().Unix(),
		ExpireTime:   userInfo.Exp,
	})
	if err != nil {
		return "", commonerrors.NewInternalError(fmt.Sprintf("failed to upsert user token: %v", err))
	}
	return sessionId, nil
}

// oauth2Config creates and returns an OAuth2 configuration for the sso.secret_path provider
func (c *ssoToken) oauth2Config(provider *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.clientId,
		ClientSecret: c.clientSecret,
		Endpoint:     provider.Endpoint(),
		Scopes:       DefaultOIDCScopes,
		RedirectURL:  c.redirectURI,
	}
}

// LoginAuthURL returns the authorization URL of the sso.login provider, for the console's client and
// redirect address, asking for PKCE: the console adds its state and code challenge. Empty when sso.login
// is off or the provider cannot be discovered.
func (c *ssoToken) LoginAuthURL() string {
	if c == nil || c.login == nil {
		return ""
	}
	provider, _, err := c.login.get()
	if err != nil {
		klog.ErrorS(err, "sso.login: no authorization URL")
		return ""
	}
	q := url.Values{
		"client_id":             {c.login.clientId},
		"redirect_uri":          {c.login.redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(DefaultOIDCScopes, " ")},
		"code_challenge_method": {"S256"},
	}
	return provider.Endpoint().AuthURL + "?" + q.Encode()
}

// AuthURL returns the OAuth2 authorization endpoint URL for SSO authentication at the provider of
// sso.secret_path: a code it returns is redeemed by POST /login without a verifier (the console's own
// sign-in unless sso.login is on, and the web apps that bridge through the console). Empty when that
// provider is not configured.
func (c *ssoToken) AuthURL() string {
	if c.provider == nil && c.secretLazy == nil {
		return ""
	}
	provider, _, err := c.secret()
	if err != nil {
		klog.ErrorS(err, "sso: no authorization URL")
		return ""
	}
	redirectUri := url.QueryEscape(c.redirectURI)
	scope := url.QueryEscape(strings.Join(DefaultOIDCScopes, " "))
	return fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code&scope=%s",
		provider.Endpoint().AuthURL, c.clientId, redirectUri, scope)
}

// generateSSOUserId generates a unique user ID based on sub or email
func generateSSOUserId(sub, email string) string {
	name := email
	if name == "" {
		name = sub
	}
	if name == "" {
		return ""
	}
	return commonuser.GenerateUserIdByName(name)
}
