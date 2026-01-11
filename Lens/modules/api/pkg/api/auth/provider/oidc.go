// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// OIDCProvider implements AuthProvider for standard OIDC authentication
// It can connect to any OIDC-compliant IdP (Okta, Azure AD, Keycloak, etc.)
type OIDCProvider struct {
	name         string
	displayName  string
	enabled      bool
	issuer       string
	clientID     string
	clientSecret string
	redirectURI  string
	scopes       []string

	// Cached discovery document
	discovery *OIDCDiscovery
}

// OIDCDiscovery represents OIDC discovery document
type OIDCDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

// NewOIDCProvider creates a new OIDC provider from database config
func NewOIDCProvider(cfg *model.LensAuthProviders) (*OIDCProvider, error) {
	displayName := cfg.Name
	if dn, ok := cfg.Config["display_name"].(string); ok && dn != "" {
		displayName = dn
	}

	issuer, ok := cfg.Config["endpoint"].(string)
	if !ok || issuer == "" {
		return nil, fmt.Errorf("OIDC endpoint is required")
	}

	clientID, ok := cfg.Config["clientId"].(string)
	if !ok || clientID == "" {
		return nil, fmt.Errorf("OIDC client_id is required")
	}

	clientSecret, _ := cfg.Config["clientSecret"].(string)
	redirectURI, _ := cfg.Config["redirectUri"].(string)

	scopes := []string{"openid", "profile", "email"}
	if s, ok := cfg.Config["scopes"].([]interface{}); ok {
		scopes = make([]string, len(s))
		for i, v := range s {
			scopes[i] = fmt.Sprintf("%v", v)
		}
	}

	p := &OIDCProvider{
		name:         cfg.Name,
		displayName:  displayName,
		enabled:      cfg.Enabled,
		issuer:       issuer,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		scopes:       scopes,
	}

	// Fetch discovery document
	if err := p.fetchDiscovery(); err != nil {
		log.Warnf("Failed to fetch OIDC discovery for %s: %v", cfg.Name, err)
		// Don't fail, discovery can be fetched later
	}

	return p, nil
}

func (p *OIDCProvider) Name() string        { return p.name }
func (p *OIDCProvider) Type() string        { return "oidc" }
func (p *OIDCProvider) DisplayName() string { return p.displayName }
func (p *OIDCProvider) IsEnabled() bool     { return p.enabled }

// NeedsExternalRedirect returns true because OIDC uses external IdP redirect
func (p *OIDCProvider) NeedsExternalRedirect() bool { return true }

// GetAuthorizeURL returns the external IdP's authorization URL
func (p *OIDCProvider) GetAuthorizeURL(req *AuthorizeRequest) (string, error) {
	if err := p.ensureDiscovery(); err != nil {
		return "", err
	}

	redirectURI := req.RedirectURI
	if redirectURI == "" {
		redirectURI = p.redirectURI
	}

	params := url.Values{
		"client_id":     {p.clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(p.scopes, " ")},
		"state":         {req.State},
	}
	if req.Nonce != "" {
		params.Set("nonce", req.Nonce)
	}

	return fmt.Sprintf("%s?%s", p.discovery.AuthorizationEndpoint, params.Encode()), nil
}

// HandleCallback exchanges authorization code for tokens and gets user info
func (p *OIDCProvider) HandleCallback(ctx context.Context, params map[string]string) (*UserInfo, error) {
	code := params["code"]
	if code == "" {
		return nil, fmt.Errorf("authorization code not found")
	}

	redirectURI := params["redirect_uri"]
	if redirectURI == "" {
		redirectURI = p.redirectURI
	}

	if err := p.ensureDiscovery(); err != nil {
		return nil, err
	}

	// Exchange code for tokens
	tokenResp, err := p.exchangeCode(ctx, code, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Get user info
	userInfo, err := p.getUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	return userInfo, nil
}

// ValidateToken validates an access token by calling userinfo endpoint
func (p *OIDCProvider) ValidateToken(ctx context.Context, token string) (*UserInfo, error) {
	if err := p.ensureDiscovery(); err != nil {
		return nil, err
	}

	return p.getUserInfo(ctx, token)
}

// fetchDiscovery fetches the OIDC discovery document
func (p *OIDCProvider) fetchDiscovery() error {
	discoveryURL := fmt.Sprintf("%s/.well-known/openid-configuration", strings.TrimSuffix(p.issuer, "/"))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", discoveryURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery request failed with status %d", resp.StatusCode)
	}

	var discovery OIDCDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return err
	}

	p.discovery = &discovery
	return nil
}

// ensureDiscovery ensures discovery document is loaded
func (p *OIDCProvider) ensureDiscovery() error {
	if p.discovery != nil {
		return nil
	}
	return p.fetchDiscovery()
}

// exchangeCode exchanges authorization code for tokens
func (p *OIDCProvider) exchangeCode(ctx context.Context, code, redirectURI string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
		"client_id":    {p.clientID},
	}
	if p.clientSecret != "" {
		data.Set("client_secret", p.clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.discovery.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token request failed: %s", string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

// getUserInfo gets user info from userinfo endpoint
func (p *OIDCProvider) getUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.discovery.UserinfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo request failed with status %d", resp.StatusCode)
	}

	var claims map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, err
	}

	userInfo := &UserInfo{}

	// Extract standard claims
	if sub, ok := claims["sub"].(string); ok {
		userInfo.ID = sub
		userInfo.Username = sub
	}
	if name, ok := claims["name"].(string); ok {
		userInfo.DisplayName = name
	}
	if email, ok := claims["email"].(string); ok {
		userInfo.Email = email
	}
	if preferredUsername, ok := claims["preferred_username"].(string); ok {
		userInfo.Username = preferredUsername
	}

	return userInfo, nil
}
