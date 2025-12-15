/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
)

var (
	ssoInitOnce sync.Once
	ssoInstance *ssoToken

	DefaultOIDCScopes = []string{oidc.ScopeOpenID, "profile", "email", oidc.ScopeOfflineAccess}
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
	// Validate required configuration
	if ssoTokenInstance.endpoint == "" || ssoTokenInstance.clientId == "" ||
		ssoTokenInstance.clientSecret == "" || ssoTokenInstance.redirectURI == "" {
		return nil, fmt.Errorf("failed to find sso config")
	}

	ctx := oidc.ClientContext(context.Background(), ssoTokenInstance.httpClient.GetBaseClient())
	var err error
	ssoTokenInstance.provider, err = oidc.NewProvider(ctx, ssoTokenInstance.endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to new provider %q: %v", ssoTokenInstance.endpoint, err)
	}
	// Configure ID token verifier
	ssoTokenInstance.verifier = ssoTokenInstance.provider.Verifier(
		&oidc.Config{ClientID: ssoTokenInstance.clientId})
	return ssoTokenInstance, nil
}

// Login exchanges authorization code for access token and ID token
// Implements TokenInterface.Login method for OAuth2 flow
func (c *ssoToken) Login(ctx context.Context, input TokenInput) (*v1.User, *TokenResponse, error) {
	if input.Code == "" {
		return nil, nil, commonerrors.NewBadRequest("no code in request")
	}
	if c.httpClient == nil {
		return nil, nil, commonerrors.NewInternalError("http client is nil")
	}
	var (
		err   error
		token *oauth2.Token
	)
	ctx = oidc.ClientContext(ctx, c.httpClient.GetBaseClient())
	config := c.oauth2Config()
	token, err = config.Exchange(ctx, input.Code)
	if err != nil {
		return nil, nil, commonerrors.NewInternalError(fmt.Sprintf("failed to get token: %v", err))
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, nil, commonerrors.NewInternalError("no id_token in token response")
	}

	// Validate ID token and extract user info
	userInfo, err := c.validate(ctx, rawIDToken)
	if err != nil {
		return nil, nil, err
	}

	// Synchronize user with system
	user, err := c.synchronizeUser(ctx, userInfo)
	if err != nil {
		return nil, nil, err
	}
	userToken := ""
	if commonconfig.IsDBEnable() {
		if userToken, err = c.updateUserInfoInDB(ctx, rawIDToken, token.RefreshToken, userInfo); err != nil {
			return nil, nil, err
		}
	} else {
		userToken = rawIDToken
	}
	response := &TokenResponse{
		Expire: userInfo.Exp,
		Token:  userToken,
	}
	return user, response, nil
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
	return c.validate(ctx, rawToken)
}

// validate verifies ID token and extracts user information
// Implements TokenInterface.Validate method for OAuth2 tokens
func (c *ssoToken) validate(ctx context.Context, rawToken string) (*UserInfo, error) {
	idToken, err := c.verifier.Verify(ctx, rawToken)
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
	return userInfo, nil
}

// synchronizeUser creates or updates user based on OAuth2 user information
func (c *ssoToken) synchronizeUser(ctx context.Context, userInfo *UserInfo) (*v1.User, error) {
	user, err := getUserById(ctx, c.Client, userInfo.Id)
	if err == nil {
		patch := client.MergeFrom(user.DeepCopy())
		isChanged := false
		if v1.GetUserName(user) != userInfo.Name {
			metav1.SetMetaDataAnnotation(&user.ObjectMeta, v1.UserNameAnnotation, userInfo.Name)
			isChanged = true
		}
		if v1.GetUserEmail(user) != userInfo.Email {
			metav1.SetMetaDataAnnotation(&user.ObjectMeta, v1.UserEmailAnnotation, userInfo.Email)
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
					v1.UserNameAnnotation:  userInfo.Name,
					v1.UserEmailAnnotation: userInfo.Email,
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
func (c *ssoToken) updateUserInfoInDB(ctx context.Context, rawIDToken string, refreshToken string, userInfo *UserInfo) (string, error) {
	sessionId := string(uuid.NewUUID())
	err := c.dbClient.UpsertUserToken(ctx, &dbclient.UserToken{
		UserId:       userInfo.Id,
		SessionId:    sessionId,
		Token:        rawIDToken,
		RefreshToken: refreshToken,
		CreationTime: time.Now().UTC().Unix(),
		ExpireTime:   userInfo.Exp,
	})
	if err != nil {
		return "", commonerrors.NewInternalError(fmt.Sprintf("failed to upsert user token: %v", err))
	}
	return sessionId, nil
}

// oauth2Config creates and returns an OAuth2 configuration
func (c *ssoToken) oauth2Config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.clientId,
		ClientSecret: c.clientSecret,
		Endpoint:     c.provider.Endpoint(),
		Scopes:       DefaultOIDCScopes,
		RedirectURL:  c.redirectURI,
	}
}

// AuthURL returns the OAuth2 authorization endpoint URL for SSO authentication
func (c *ssoToken) AuthURL() string {
	redirectUri := url.QueryEscape(c.redirectURI)
	scope := url.QueryEscape(strings.Join(DefaultOIDCScopes, " "))
	return fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code&scope=%s",
		c.provider.Endpoint().AuthURL, c.clientId, redirectUri, scope)
}

// RefreshWithOAuth2 uses the stored refresh token to obtain a new ID token and updates the database.
func (c *ssoToken) RefreshWithOAuth2(ctx context.Context, userToken *dbclient.UserToken) (*dbclient.UserToken, error) {
	if userToken.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available for user %s", userToken.UserId)
	}

	ctx = oidc.ClientContext(ctx, c.httpClient.GetBaseClient())
	config := c.oauth2Config()
	tokenSource := config.TokenSource(ctx, &oauth2.Token{
		RefreshToken: userToken.RefreshToken,
	})

	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token for user %s: %v", userToken.UserId, err)
	}

	rawIDToken, ok := newToken.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in refreshed token response for user %s", userToken.UserId)
	}

	userInfo, err := c.validate(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to validate refreshed ID token for user %s: %v", userToken.UserId, err)
	}

	updatedToken := &dbclient.UserToken{
		UserId:       userToken.UserId,
		SessionId:    userToken.SessionId,
		Token:        rawIDToken,
		CreationTime: time.Now().UTC().Unix(),
		ExpireTime:   userInfo.Exp,
	}

	// Handle refresh token rotation: if a new refresh token is provided, use it.
	if newToken.RefreshToken != "" {
		updatedToken.RefreshToken = newToken.RefreshToken
	} else {
		updatedToken.RefreshToken = userToken.RefreshToken // Keep the old one if no new one is provided
	}

	err = c.dbClient.UpsertUserToken(ctx, updatedToken)
	if err != nil {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to upsert refreshed user token: %v", err))
	}

	return updatedToken, nil
}

// GetDBClient returns the database client instance
func (c *ssoToken) GetDBClient() dbclient.Interface {
	return c.dbClient
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
