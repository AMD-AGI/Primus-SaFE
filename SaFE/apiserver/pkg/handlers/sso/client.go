/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package sso

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

var (
	once     sync.Once
	instance *SsoClient

	scopes = []string{"openid", "profile", "email", "groups", "federated:id"}
)

type UserData struct {
	// 内部生成的id，用于唯一标识用户
	Id string `json:"id,omitempty"`
	// 用户名
	Name string `json:"name,omitempty"`
	// A locally unique and never reassigned identifier within the Issuer for the End-User,
	Sub string `json:"sub,omitempty"`
	// token到期时间
	Exp    int64           `json:"exp,omitempty"`
	Email  string          `json:"email,omitempty"`
	Claims FederatedClaims `json:"federated_claims"`
}

type FederatedClaims struct {
	ConnectorId string `json:"connector_id"`
	UserId      string `json:"user_id"`
}

type SsoClient struct {
	*config

	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider

	// Does the provider use "offline_access" scope to request a refresh token
	// or does it use "access_type=offline" (e.g. Google)?
	offlineAsScope bool

	httpClient httpclient.Interface
}

func NewSsoClient() *SsoClient {
	once.Do(func() {
		instance, _ = newClient()
	})
	return instance
}

func newClient() (*SsoClient, error) {
	c, err := initConfig()
	if err != nil {
		klog.ErrorS(err, "failed to init oidc config")
		return nil, err
	}
	cli := &SsoClient{
		config: c,
	}
	if cli.httpClient = httpclient.NewClientWithTimeout(time.Second * 10); cli.httpClient == nil {
		klog.ErrorS(err, "failed to new http client")
		return nil, err
	}
	if err = cli.newProvider(); err != nil {
		klog.ErrorS(err, "failed to new provider")
		return nil, err
	}
	return cli, nil
}

func (c *SsoClient) newProvider() error {
	ctx := oidc.ClientContext(context.Background(), c.httpClient.GetBaseClient())
	provider, err := oidc.NewProvider(ctx, c.endpoint)
	if err != nil {
		return fmt.Errorf("failed to new provider %q: %v", c.endpoint, err)
	}

	var s struct {
		// What scopes does a provider support?
		// See: https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderMetadata
		ScopesSupported []string `json:"scopes_supported"`
	}
	if err = provider.Claims(&s); err != nil {
		return fmt.Errorf("failed to parse provider scopes_supported: %v", err)
	}

	if len(s.ScopesSupported) == 0 {
		// scopes_supported is a "RECOMMENDED" discovery claim, not a required
		// one. If missing, assume that the provider follows the spec and has
		// an "offline_access" scope.
		c.offlineAsScope = true
	} else {
		// See if scopes_supported has the "offline_access" scope.
		c.offlineAsScope = func() bool {
			for _, scope := range s.ScopesSupported {
				if scope == oidc.ScopeOfflineAccess {
					return true
				}
			}
			return false
		}()
	}

	c.provider = provider
	c.verifier = provider.Verifier(&oidc.Config{ClientID: c.id})
	return nil
}

func (c *SsoClient) GetAuthUrl(redirectURL, state string) string {
	return c.oauth2Config(c.getScopes(), redirectURL).AuthCodeURL(state)
}

func (c *SsoClient) GetAuthUrlWithConnector(redirectURL, state string, connectorID string) string {
	authURL := c.oauth2Config(c.getScopes(), redirectURL).AuthCodeURL(state)
	// 解析 URL
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		// 处理解析错误
		fmt.Println("Error parsing URL:", err)
		return authURL
	}
	// 拼接 connector_id 到路径末尾
	parsedURL.Path = parsedURL.Path + "/" + connectorID
	// 返回修改后的 URL
	return parsedURL.String()
}

func (c *SsoClient) Login(w http.ResponseWriter, r *http.Request, redirectURL string) error {
	authCodeURL := c.GetAuthUrl(redirectURL, "")
	r.Method = http.MethodGet
	http.Redirect(w, r, authCodeURL, http.StatusSeeOther)
	return nil
}

func (c *SsoClient) GetToken(ctx context.Context, code, redirectURL string) (string, *UserData, error) {
	if code == "" {
		return "", nil, commonerrors.NewBadRequest("no code in request")
	}
	var (
		err   error
		token *oauth2.Token
	)
	ctx = oidc.ClientContext(ctx, c.httpClient.GetBaseClient())

	oauth2Config := c.oauth2Config(c.getScopes(), redirectURL)

	token, err = oauth2Config.Exchange(ctx, code)
	if err != nil {
		return "", nil, commonerrors.NewInternalError(fmt.Sprintf("failed to get token: %v", err))
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", nil, commonerrors.NewInternalError("no id_token in token response")
	}

	userInfo, err := c.ValidateToken(ctx, rawIDToken)
	if err != nil {
		return "", nil, err
	}
	klog.Infof("user id: %s, email: %s, code: %s", userInfo.Id, userInfo.Email, code)
	return rawIDToken, userInfo, nil
}

func (c *SsoClient) ValidateToken(ctx context.Context, tokenString string) (*UserData, error) {
	idToken, err := c.verifier.Verify(ctx, tokenString)
	if err != nil {
		return nil, fmt.Errorf("fail to verify ID token: %v", err)
	}

	var claims json.RawMessage
	if err = idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("fail to decode ID token claims: %v", err)
	}

	buff := new(bytes.Buffer)
	if err = json.Indent(buff, claims, "", "  "); err != nil {
		return nil, fmt.Errorf("fail to indent ID token claims: %v", err)
	}
	klog.Infof("user buffer: %s", buff.String())
	data := &UserData{}
	err = json.Unmarshal(buff.Bytes(), data)
	if err != nil {
		return nil, err
	}
	data.Id = GenUserId(data.Sub, data.Email)
	return data, nil
}

func (c *SsoClient) oauth2Config(scopes []string, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.id,
		ClientSecret: c.secret,
		Endpoint:     c.provider.Endpoint(),
		Scopes:       scopes,
		RedirectURL:  redirectURL,
	}
}

func (c *SsoClient) getScopes() []string {
	scopes2 := slice.Copy(scopes, len(scopes))
	if c.offlineAsScope {
		scopes2 = append(scopes2, "offline_access")
	}
	return scopes2
}

func GenUserId(sub, email string) string {
	id := email
	if id == "" {
		id = sub
	}
	if id == "" {
		return ""
	}
	return strings.ToLower(stringutil.MD5(id))
}
