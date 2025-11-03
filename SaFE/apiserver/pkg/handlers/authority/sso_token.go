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

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
)

var (
	ssoInitOnce sync.Once
	ssoInstance *ssoToken

	DefaultOIDCScopes = []string{oidc.ScopeOpenID, "profile", "email", "groups"}
)

// ssoToken implements TokenInterface for OAuth2/OpenID Connect authentication
type ssoToken struct {
	endpoint     string
	clientId     string
	clientSecret string
	redirectURI  string

	client.Client
	httpClient httpclient.Interface
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
	ssoTokenInstance := &ssoToken{
		endpoint:     commonconfig.GetSSOEndpoint(),
		clientId:     commonconfig.GetSSOClientId(),
		clientSecret: commonconfig.GetSSOClientSecret(),
		redirectURI:  commonconfig.GetSSORedirectURI(),
		Client:       cli,
		httpClient:   httpclient.NewClient(),
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
	userInfo, err := c.Validate(ctx, rawIDToken)
	if err != nil {
		return nil, nil, err
	}
	klog.Infof("user id: %s, email: %s, code: %s", userInfo.Id, userInfo.Email, input.Code)

	// Synchronize user with system
	user, err := c.synchronizeUser(ctx, userInfo)
	if err != nil {
		return nil, nil, err
	}
	response := &TokenResponse{
		Expire:   userInfo.Exp,
		RawToken: rawIDToken,
	}
	return user, response, nil
}

// Validate verifies ID token and extracts user information
// Implements TokenInterface.Validate method for OAuth2 tokens
func (c *ssoToken) Validate(ctx context.Context, rawToken string) (*UserInfo, error) {
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
	// klog.Infof("user buffer: %s", buff.String())
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
				Type: v1.SSOUser,
			},
		}
		if err = c.Create(ctx, user); err != nil {
			return nil, err
		}
	}
	return user, err
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
